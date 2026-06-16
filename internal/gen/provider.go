package gen

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// 지원 프로바이더 식별자
const (
	ProviderGemini     = "gemini"
	ProviderOpenAI     = "openai"
	ProviderOpenRouter = "openrouter"
	ProviderFal        = "fal"
	ProviderBytePlus   = "byteplus"
)

// SupportedProviders는 지원 프로바이더 식별자 목록입니다 (UI 노출 순서).
var SupportedProviders = []string{ProviderGemini, ProviderOpenAI, ProviderOpenRouter, ProviderFal, ProviderBytePlus}

// modelCatalog는 프로바이더별 선택 가능한 이미지 모델 목록입니다 (최신 모델이 맨 앞).
var modelCatalog = map[string][]string{
	ProviderGemini: {
		"gemini-3-pro-image", // Nano Banana Pro (최신)
		"gemini-3-pro-image-preview",
		"gemini-2.5-flash-image", // Nano Banana
	},
	ProviderOpenAI: {
		"gpt-image-2",
		"gpt-image-1.5",
		"gpt-image-1",
		"gpt-image-1-mini",
	},
	ProviderOpenRouter: {
		"google/gemini-3-pro-image-preview", // 최신
		"google/gemini-2.5-flash-image",
		"google/gemini-2.5-flash-image-preview",
	},
	ProviderFal: {
		"fal-ai/nano-banana-pro", // 최신
		"fal-ai/nano-banana",
		"fal-ai/flux-pro/v1.1-ultra",
		"fal-ai/flux/dev",
	},
	ProviderBytePlus: {
		"seedream-4-0-250828",     // Seedream 4.0 (최신)
		"seedream-3-0-t2i-250415", // Seedream 3.0
		"seededit-3-0-i2i-250628", // SeedEdit 3.0 (이미지 편집)
	},
}

// ModelsFor는 프로바이더가 제공하는 선택 가능한 모델 목록을 반환합니다 (최신 모델이 맨 앞).
func ModelsFor(provider string) []string {
	if list, ok := modelCatalog[provider]; ok {
		return append([]string(nil), list...)
	}
	return nil
}

// Provider는 이미지 생성 백엔드 공통 인터페이스입니다.
type Provider interface {
	// GenerateImage는 프롬프트와 참조 이미지(PNG)로 이미지를 생성합니다.
	GenerateImage(ctx context.Context, prompt string, refImages [][]byte, aspectRatio string) ([]byte, error)
	// ValidateKey는 API 키 유효성을 확인합니다.
	ValidateKey(ctx context.Context) error
}

// DefaultModelFor는 프로바이더별 기본 모델을 반환합니다.
func DefaultModelFor(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return "gpt-image-2"
	case ProviderOpenRouter:
		return "google/gemini-3-pro-image-preview"
	case ProviderFal:
		return "fal-ai/nano-banana-pro"
	case ProviderBytePlus:
		return "seedream-4-0-250828"
	default:
		return DefaultModel // gemini-3-pro-image (Nano Banana Pro)
	}
}

// ProviderLabel은 UI 표시용 이름입니다.
func ProviderLabel(provider string) string {
	switch provider {
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderOpenRouter:
		return "OpenRouter"
	case ProviderFal:
		return "fal.ai"
	case ProviderBytePlus:
		return "BytePlus"
	default:
		return "Gemini"
	}
}

// New는 프로바이더 구현체를 생성합니다.
func New(provider, apiKey, model string) (Provider, error) {
	if model == "" {
		model = DefaultModelFor(provider)
	}
	switch provider {
	case ProviderGemini, "":
		return NewClient(apiKey, model), nil
	case ProviderOpenAI:
		return NewOpenAI(apiKey, model), nil
	case ProviderOpenRouter:
		return NewOpenRouter(apiKey, model), nil
	case ProviderFal:
		return NewFal(apiKey, model), nil
	case ProviderBytePlus:
		return NewBytePlus(apiKey, model), nil
	default:
		return nil, fmt.Errorf("지원하지 않는 프로바이더입니다: %s", provider)
	}
}

// aspectHint는 종횡비 파라미터를 지원하지 않는 API용 프롬프트 보조 문구입니다.
func aspectHint(aspectRatio string) string {
	switch aspectRatio {
	case "", "1:1":
		return "Render on a square 1:1 canvas."
	case "16:9":
		return "Render on a wide 16:9 landscape canvas, much wider than tall."
	case "21:9":
		return "Render on an extra-wide 21:9 panorama canvas, with plenty of horizontal room for a long row of poses."
	default:
		return fmt.Sprintf("Render on a wide %s landscape canvas.", aspectRatio)
	}
}

// decodeDataOrDownload는 data: URL이면 디코딩하고, http(s) URL이면 다운로드합니다.
func decodeDataOrDownload(httpClient *http.Client, url string) ([]byte, error) {
	if strings.HasPrefix(url, "data:") {
		idx := strings.Index(url, "base64,")
		if idx < 0 {
			return nil, errors.New("이미지 데이터 형식을 해석할 수 없습니다")
		}
		return base64.StdEncoding.DecodeString(url[idx+7:])
	}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("결과 이미지 다운로드 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("결과 이미지 다운로드 실패 (HTTP %d)", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}
