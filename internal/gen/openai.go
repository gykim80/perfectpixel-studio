package gen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

const (
	openAIImageGenerationEndpoint = "https://api.openai.com/v1/images/generations"
	openAIImageEditEndpoint       = "https://api.openai.com/v1/images/edits"
	openAIModelsEndpoint          = "https://api.openai.com/v1/models"
)

// OpenAI는 OpenAI Image API 클라이언트입니다.
type OpenAI struct {
	APIKey string
	Model  string
	HTTP   *http.Client

	generationEndpoint string // 테스트용 오버라이드
	editEndpoint       string // 테스트용 오버라이드
	modelsEndpoint     string // 테스트용 오버라이드
}

// NewOpenAI는 새 OpenAI 이미지 생성 클라이언트를 생성합니다.
func NewOpenAI(apiKey, model string) *OpenAI {
	if model == "" {
		model = DefaultModelFor(ProviderOpenAI)
	}
	return &OpenAI{
		APIKey: apiKey,
		Model:  model,
		HTTP:   &http.Client{Timeout: 300 * time.Second},
	}
}

type openAIImageRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	N            int    `json:"n"`
	Size         string `json:"size,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
}

type openAIImageResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// GenerateImage는 OpenAI GPT Image 모델로 이미지를 생성하거나 참조 이미지를 편집합니다.
func (c *OpenAI) GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("OpenAI API 키가 설정되지 않았습니다. 설정에서 입력해 주세요")
	}

	fullPrompt := prompt + "\n\n" + aspectHint(aspectRatio)
	size := openAISizeFor(aspectRatio)

	var body []byte
	var contentType string
	var endpoint string
	var err error
	if len(refImages) == 0 {
		endpoint = c.generationEndpoint
		if endpoint == "" {
			endpoint = openAIImageGenerationEndpoint
		}
		body, err = json.Marshal(openAIImageRequest{
			Model:        c.Model,
			Prompt:       fullPrompt,
			N:            1,
			Size:         size,
			Quality:      "medium",
			OutputFormat: "png",
		})
		contentType = "application/json"
	} else {
		endpoint = c.editEndpoint
		if endpoint == "" {
			endpoint = openAIImageEditEndpoint
		}
		body, contentType, err = c.buildEditBody(fullPrompt, refImages, size)
	}
	if err != nil {
		return nil, fmt.Errorf("요청 직렬화 실패: %w", err)
	}

	var lastErr error
	backoff := 2 * time.Second
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		img, retryable, err := c.doImageRequest(ctx, endpoint, contentType, body)
		if err == nil {
			return img, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *OpenAI) buildEditBody(prompt string, refImages [][]byte, size string) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := map[string]string{
		"model":         c.Model,
		"prompt":        prompt,
		"n":             "1",
		"size":          size,
		"quality":       "medium",
		"output_format": "png",
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	for i, img := range refImages {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="reference-%d.png"`, i+1))
		h.Set("Content-Type", "image/png")
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(img); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func (c *OpenAI) doImageRequest(ctx context.Context, endpoint, contentType string, body []byte) (img []byte, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("네트워크 오류: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, true, fmt.Errorf("응답 읽기 실패: %w", err)
	}

	var parsed openAIImageResponse
	_ = json.Unmarshal(respBytes, &parsed)

	if resp.StatusCode != http.StatusOK {
		retryable = resp.StatusCode == 429 || resp.StatusCode >= 500
		if parsed.Error != nil && parsed.Error.Message != "" {
			return nil, retryable, fmt.Errorf("OpenAI 오류 (%d): %s", resp.StatusCode, parsed.Error.Message)
		}
		return nil, retryable, fmt.Errorf("OpenAI 오류 (HTTP %d)", resp.StatusCode)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, true, fmt.Errorf("OpenAI 오류: %s", parsed.Error.Message)
	}
	if len(parsed.Data) == 0 {
		return nil, true, errors.New("응답에 이미지가 없습니다")
	}
	d := parsed.Data[0]
	if d.B64JSON != "" {
		data, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return nil, false, fmt.Errorf("이미지 디코딩 실패: %w", err)
		}
		return data, false, nil
	}
	if d.URL != "" {
		data, err := decodeDataOrDownload(c.HTTP, d.URL)
		if err != nil {
			return nil, false, err
		}
		return data, false, nil
	}
	return nil, true, errors.New("응답에 이미지가 없습니다")
}

// ValidateKey는 OpenAI 키 유효성을 확인합니다.
func (c *OpenAI) ValidateKey(ctx context.Context) error {
	key := strings.TrimSpace(c.APIKey)
	if !strings.HasPrefix(key, "sk-") || len(key) < 20 {
		return errors.New("OpenAI API 키 형식이 올바르지 않습니다")
	}
	ep := c.modelsEndpoint
	if ep == "" {
		ep = openAIModelsEndpoint + "/" + c.Model
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("네트워크 오류: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("API 키가 유효하지 않거나 이 모델 권한이 없습니다")
	}
	if resp.StatusCode == 404 {
		return errors.New("OpenAI 계정에서 GPT Image 모델을 찾을 수 없습니다")
	}
	return fmt.Errorf("키 확인 실패 (HTTP %d)", resp.StatusCode)
}

// openAISizeFor는 GPT Image 2가 허용하는 WxH 해상도로 종횡비를 변환합니다.
func openAISizeFor(aspectRatio string) string {
	w, h := parseAspect(aspectRatio)
	if w <= 0 || h <= 0 {
		return "1024x1024"
	}
	if w == h {
		return "1024x1024"
	}
	const maxEdge = 1792
	var pw, ph int
	if w > h {
		pw = maxEdge
		ph = int(float64(maxEdge) * float64(h) / float64(w))
	} else {
		ph = maxEdge
		pw = int(float64(maxEdge) * float64(w) / float64(h))
	}
	round16 := func(v int) int {
		if v < 640 {
			v = 640
		}
		return (v / 16) * 16
	}
	return strconv.Itoa(round16(pw)) + "x" + strconv.Itoa(round16(ph))
}
