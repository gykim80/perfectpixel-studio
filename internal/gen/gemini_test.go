package gen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// 1px PNG 대용 페이로드 (디코딩 검증은 하지 않으므로 임의 바이트면 충분)
var fakePNG = []byte{0x89, 'P', 'N', 'G', 1, 2, 3}

func imageResponse() string {
	b64 := base64.StdEncoding.EncodeToString(fakePNG)
	return fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":%q}}]},"finishReason":"STOP"}]}`, b64)
}

// TestGeminiModelFallback은 기본 모델 404 시 폴백 체인으로 자동 전환되는지 검증합니다.
func TestGeminiModelFallback(t *testing.T) {
	var calledModels []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 경로 형식: /models/<model>:generateContent
		path := strings.TrimPrefix(r.URL.Path, "/models/")
		model := strings.TrimSuffix(path, ":generateContent")
		calledModels = append(calledModels, model)
		if model == DefaultModel || model == "gemini-3-pro-image-preview" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"model not found","status":"NOT_FOUND"}}`))
			return
		}
		_, _ = w.Write([]byte(imageResponse()))
	}))
	defer srv.Close()

	c := NewClient("test-key", "")
	c.endpoint = srv.URL + "/models/%s:generateContent"

	img, err := c.GenerateImage(context.Background(), "prompt", nil, "1:1")
	if err != nil {
		t.Fatalf("폴백 생성 실패: %v", err)
	}
	if string(img) != string(fakePNG) {
		t.Fatalf("이미지 바이트 불일치")
	}
	want := []string{DefaultModel, "gemini-3-pro-image-preview", "gemini-3.1-flash-image"}
	if len(calledModels) != len(want) {
		t.Fatalf("호출 모델 시퀀스 오류: %v", calledModels)
	}
	for i := range want {
		if calledModels[i] != want[i] {
			t.Fatalf("호출 순서 오류: got %v want %v", calledModels, want)
		}
	}
}

// TestGeminiAllModelsNotFound는 모든 폴백 소진 시 오류를 반환하는지 검증합니다.
func TestGeminiAllModelsNotFound(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"nope","status":"NOT_FOUND"}}`))
	}))
	defer srv.Close()

	c := NewClient("test-key", "")
	c.endpoint = srv.URL + "/models/%s:generateContent"
	if _, err := c.GenerateImage(context.Background(), "p", nil, ""); err == nil {
		t.Fatal("모든 모델 404면 오류여야 합니다")
	}
	// 기본 + 폴백 3개 = 4회
	if calls != 1+len(modelFallbacks) {
		t.Fatalf("호출 횟수 오류: %d", calls)
	}
}

func TestGeminiTimeoutIsNotRetried(t *testing.T) {
	calls := 0
	c := NewClient("test-key", "")
	c.HTTP = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, context.DeadlineExceeded
	})}

	_, err := c.GenerateImage(context.Background(), "p", nil, "1:1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline 오류를 기대했지만 got %v", err)
	}
	if calls != 1 {
		t.Fatalf("timeout 요청이 %d회 호출됨; 재시도하면 안 됩니다", calls)
	}
}

// TestGeminiNonDefaultModelFallback은 사용자 지정 모델이 404일 때도
// 폴백 체인이 동작하는지 검증합니다.
func TestGeminiNonDefaultModelFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "my-custom-model") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"x","status":"NOT_FOUND"}}`))
			return
		}
		_, _ = w.Write([]byte(imageResponse()))
	}))
	defer srv.Close()

	c := NewClient("test-key", "my-custom-model")
	c.endpoint = srv.URL + "/models/%s:generateContent"
	if _, err := c.GenerateImage(context.Background(), "p", nil, ""); err != nil {
		t.Fatalf("사용자 모델 폴백 실패: %v", err)
	}
}

// TestGeminiRequestBody는 참조 이미지/종횡비가 요청에 올바르게 들어가는지 검증합니다.
func TestGeminiRequestBody(t *testing.T) {
	var got genRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(imageResponse()))
	}))
	defer srv.Close()

	c := NewClient("test-key", "")
	c.endpoint = srv.URL + "/models/%s:generateContent"
	if _, err := c.GenerateImage(context.Background(), "hello", [][]byte{fakePNG}, "21:9"); err != nil {
		t.Fatalf("생성 실패: %v", err)
	}
	if len(got.Contents) != 1 || len(got.Contents[0].Parts) != 2 {
		t.Fatalf("parts 구성 오류: %+v", got)
	}
	if got.Contents[0].Parts[0].InlineData == nil || got.Contents[0].Parts[1].Text != "hello" {
		t.Fatal("참조 이미지가 프롬프트보다 앞에 와야 합니다")
	}
	if got.GenerationConfig == nil || got.GenerationConfig.ImageConfig == nil ||
		got.GenerationConfig.ImageConfig.AspectRatio != "21:9" {
		t.Fatalf("aspectRatio 누락: %+v", got.GenerationConfig)
	}
	// Gemini 3 Pro + 와이드 스트립 → 2K 해상도로 프레임당 픽셀 확보
	if got.GenerationConfig.ImageConfig.ImageSize != "2K" {
		t.Fatalf("imageSize 2K 누락: %+v", got.GenerationConfig.ImageConfig)
	}
}

// TestGeminiImageSizeOnlyForPro는 폴백 모델 요청에는 imageSize가 빠지는지 검증합니다.
func TestGeminiImageSizeOnlyForPro(t *testing.T) {
	sizeByModel := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/models/")
		model := strings.TrimSuffix(path, ":generateContent")
		var req genRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.GenerationConfig != nil && req.GenerationConfig.ImageConfig != nil {
			sizeByModel[model] = req.GenerationConfig.ImageConfig.ImageSize
		}
		if strings.HasPrefix(model, "gemini-3-pro") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"x","status":"NOT_FOUND"}}`))
			return
		}
		_, _ = w.Write([]byte(imageResponse()))
	}))
	defer srv.Close()

	c := NewClient("test-key", "")
	c.endpoint = srv.URL + "/models/%s:generateContent"
	if _, err := c.GenerateImage(context.Background(), "p", nil, "21:9"); err != nil {
		t.Fatalf("생성 실패: %v", err)
	}
	if sizeByModel[DefaultModel] != "2K" {
		t.Fatalf("Pro 모델에 2K 누락: %v", sizeByModel)
	}
	if sizeByModel["gemini-3.1-flash-image"] != "" {
		t.Fatalf("폴백 모델에 imageSize가 포함됨: %v", sizeByModel)
	}
}
