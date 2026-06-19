// Package gen은 AI 이미지 생성 프로바이더 클라이언트를 제공합니다.
package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultModel은 기본 이미지 생성 모델입니다 (Nano Banana Pro / Gemini 3 Pro Image).
// 참조 이미지 정체성 유지와 복잡한 레이아웃 지시 준수가 크게 개선되어
// 스프라이트 스트립의 프레임 간 안정성에 가장 유리합니다.
const DefaultModel = "gemini-3-pro-image"

// modelFallbacks는 기본 모델이 아직 제공되지 않는 키/리전을 위한 폴백 체인입니다.
// 404(모델 없음)일 때만 순서대로 시도합니다.
var modelFallbacks = []string{
	"gemini-3-pro-image-preview",
	"gemini-3.1-flash-image",
	"gemini-2.5-flash-image",
}

var errModelNotFound = errors.New("요청한 모델을 찾을 수 없습니다")

const apiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"

// Client는 Gemini 이미지 생성 API 클라이언트입니다.
type Client struct {
	APIKey string
	Model  string
	HTTP   *http.Client

	endpoint string // 테스트용 오버라이드 (빈 값이면 apiEndpoint)
}

// NewClient는 새 Gemini 클라이언트를 생성합니다.
func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		APIKey: apiKey,
		Model:  model,
		HTTP:   &http.Client{Timeout: 180 * time.Second},
	}
}

type genRequest struct {
	Contents         []genContent `json:"contents"`
	GenerationConfig *genConfig   `json:"generationConfig,omitempty"`
}

type genContent struct {
	Parts []genPart `json:"parts"`
}

type genPart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type genConfig struct {
	ResponseModalities []string     `json:"responseModalities,omitempty"`
	ImageConfig        *imageConfig `json:"imageConfig,omitempty"`
}

type imageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ImageSize   string `json:"imageSize,omitempty"`
}

type genResponse struct {
	Candidates []struct {
		Content struct {
			Parts []genPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *apiError `json:"error"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// GenerateImage는 프롬프트와 참조 이미지(PNG 바이트)로 이미지를 생성합니다.
// aspectRatio는 "1:1", "16:9", "21:9" 등을 지원하며 빈 문자열이면 생략됩니다.
func (c *Client) GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("API 키가 설정되지 않았습니다. 설정에서 Gemini API 키를 입력해 주세요")
	}

	parts := make([]genPart, 0, len(refImages)+1)
	for _, img := range refImages {
		parts = append(parts, genPart{
			InlineData: &inlineData{
				MimeType: "image/png",
				Data:     base64.StdEncoding.EncodeToString(img),
			},
		})
	}
	parts = append(parts, genPart{Text: prompt})

	// 모델별 요청 본문: Gemini 3 Pro는 와이드 스트립에서 2K 해상도를 지원해
	// 프레임당 픽셀 수가 늘어나 추출 품질이 크게 향상됩니다.
	// 폴백 모델은 imageSize를 지원하지 않으므로 모델에 따라 본문을 다시 만듭니다.
	buildBody := func(model string) ([]byte, error) {
		cfg := &genConfig{ResponseModalities: []string{"TEXT", "IMAGE"}}
		if aspectRatio != "" {
			ic := &imageConfig{AspectRatio: aspectRatio}
			if strings.HasPrefix(model, "gemini-3-pro") && aspectRatio != "1:1" {
				ic.ImageSize = "2K"
			}
			cfg.ImageConfig = ic
		}
		return json.Marshal(genRequest{
			Contents:         []genContent{{Parts: parts}},
			GenerationConfig: cfg,
		})
	}
	reqBody, err := buildBody(c.Model)
	if err != nil {
		return nil, fmt.Errorf("요청 직렬화 실패: %w", err)
	}

	var lastErr error
	backoff := 2 * time.Second
	model := c.Model
	fallbacks := modelFallbacks
	tried := map[string]bool{model: true}
	for attempt := 0; attempt < 3; {
		img, retryable, err := c.doRequest(ctx, model, reqBody)
		if err == nil {
			return img, nil
		}
		lastErr = err

		// 모델 미제공(404) → 폴백 체인으로 즉시 교체 (시도 횟수 미차감)
		if errors.Is(err, errModelNotFound) {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			for len(fallbacks) > 0 && tried[fallbacks[0]] {
				fallbacks = fallbacks[1:]
			}
			if len(fallbacks) == 0 {
				return nil, err
			}
			model = fallbacks[0]
			fallbacks = fallbacks[1:]
			tried[model] = true
			if reqBody, err = buildBody(model); err != nil {
				return nil, fmt.Errorf("요청 직렬화 실패: %w", err)
			}
			continue
		}
		if !retryable {
			return nil, err
		}
		attempt++
		if attempt >= 3 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return nil, lastErr
}

func (c *Client) doRequest(ctx context.Context, model string, body []byte) (img []byte, retryable bool, err error) {
	ep := c.endpoint
	if ep == "" {
		ep = apiEndpoint
	}
	url := fmt.Sprintf(ep, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		// http.Client.Timeout도 context.DeadlineExceeded로 감싸져 온다. 이미 한 번
		// 제한 시간을 모두 쓴 요청을 재시도하면 UI가 최대 3배 오래 멈춰 보인다.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, false, err
		}
		return nil, true, fmt.Errorf("네트워크 오류: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, true, fmt.Errorf("응답 읽기 실패: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, false, fmt.Errorf("모델 %q: %w", model, errModelNotFound)
		}
		retryable = resp.StatusCode == 429 || resp.StatusCode >= 500
		var parsed genResponse
		if json.Unmarshal(respBytes, &parsed) == nil && parsed.Error != nil {
			return nil, retryable, fmt.Errorf("Gemini API 오류 (%d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, retryable, fmt.Errorf("Gemini API 오류 (HTTP %d)", resp.StatusCode)
	}

	var parsed genResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, false, fmt.Errorf("응답 파싱 실패: %w", err)
	}
	if len(parsed.Candidates) == 0 {
		return nil, true, errors.New("이미지 생성 결과가 비어 있습니다")
	}
	for _, part := range parsed.Candidates[0].Content.Parts {
		if part.InlineData != nil && part.InlineData.Data != "" {
			data, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if err != nil {
				return nil, false, fmt.Errorf("이미지 디코딩 실패: %w", err)
			}
			return data, false, nil
		}
	}
	reason := parsed.Candidates[0].FinishReason
	return nil, true, fmt.Errorf("응답에 이미지가 없습니다 (사유: %s)", reason)
}

// ValidateKey는 API 키 유효성을 가볍게 확인합니다.
func (c *Client) ValidateKey(ctx context.Context) error {
	url := "https://generativelanguage.googleapis.com/v1beta/models?pageSize=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-goog-api-key", c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("네트워크 오류: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == 400 || resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("API 키가 유효하지 않습니다")
	}
	return fmt.Errorf("키 확인 실패 (HTTP %d)", resp.StatusCode)
}
