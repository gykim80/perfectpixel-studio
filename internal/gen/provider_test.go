package gen

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewFactory(t *testing.T) {
	cases := []struct {
		provider string
		wantType string
	}{
		{"gemini", "*gen.Client"},
		{"", "*gen.Client"},
		{"openai", "*gen.OpenAI"},
		{"openrouter", "*gen.OpenRouter"},
		{"fal", "*gen.Fal"},
		{"byteplus", "*gen.BytePlus"},
	}
	for _, c := range cases {
		p, err := New(c.provider, "test-key", "")
		if err != nil {
			t.Fatalf("[%s] 팩토리 오류: %v", c.provider, err)
		}
		if got := typeName(p); got != c.wantType {
			t.Fatalf("[%s] 타입 오류: got %s want %s", c.provider, got, c.wantType)
		}
	}
	if _, err := New("unknown", "k", ""); err == nil {
		t.Fatal("알 수 없는 프로바이더는 오류여야 합니다")
	}
}

func typeName(v any) string {
	switch v.(type) {
	case *Client:
		return "*gen.Client"
	case *OpenAI:
		return "*gen.OpenAI"
	case *OpenRouter:
		return "*gen.OpenRouter"
	case *Fal:
		return "*gen.Fal"
	case *BytePlus:
		return "*gen.BytePlus"
	default:
		return "?"
	}
}

func TestDefaultModelFor(t *testing.T) {
	if DefaultModelFor("gemini") != DefaultModel {
		t.Fatal("gemini 기본 모델 오류")
	}
	if DefaultModelFor("openai") != "gpt-image-2" {
		t.Fatal("openai 기본 모델 오류")
	}
	if DefaultModelFor("openrouter") != "google/gemini-3-pro-image-preview" {
		t.Fatal("openrouter 기본 모델 오류")
	}
	if DefaultModelFor("fal") != "fal-ai/nano-banana-pro" {
		t.Fatal("fal 기본 모델 오류")
	}
	if DefaultModelFor("byteplus") != "seedream-4-0-250828" {
		t.Fatal("byteplus 기본 모델 오류")
	}
}

func TestModelsFor(t *testing.T) {
	for _, p := range SupportedProviders {
		models := ModelsFor(p)
		if len(models) == 0 {
			t.Fatalf("[%s] 모델 목록이 비어 있습니다", p)
		}
		// 최신(맨 앞) 모델은 기본 모델과 일치해야 합니다
		if models[0] != DefaultModelFor(p) {
			t.Fatalf("[%s] 최신 모델이 기본 모델과 다릅니다: %s != %s", p, models[0], DefaultModelFor(p))
		}
	}
	if ModelsFor("unknown") != nil {
		t.Fatal("알 수 없는 프로바이더는 nil이어야 합니다")
	}
}

func TestBPSizeFor(t *testing.T) {
	if got := bpSizeFor("1:1"); got != "4096x4096" && got != "2048x2048" {
		t.Fatalf("정사각 크기 오류: %s", got)
	}
	if got := bpSizeFor(""); got != "2048x2048" {
		t.Fatalf("빈 종횡비 기본 크기 오류: %s", got)
	}
	// 와이드 스트립: 긴 변 4096, 짧은 변은 최소 1280으로 클램프
	if got := bpSizeFor("7:1"); got != "4096x1280" {
		t.Fatalf("와이드 크기 클램프 오류: %s", got)
	}
}

func TestOpenAISizeFor(t *testing.T) {
	if got := openAISizeFor("1:1"); got != "1024x1024" {
		t.Fatalf("정사각 크기 오류: %s", got)
	}
	if got := openAISizeFor("16:9"); got != "1792x1008" {
		t.Fatalf("16:9 크기 오류: %s", got)
	}
	if got := openAISizeFor("21:9"); got != "1792x768" {
		t.Fatalf("21:9 크기 오류: %s", got)
	}
}

func TestFalEndpoint(t *testing.T) {
	c := NewFal("k", "fal-ai/nano-banana")
	if got := c.endpoint(false); got != "https://fal.run/fal-ai/nano-banana" {
		t.Fatalf("기본 엔드포인트 오류: %s", got)
	}
	if got := c.endpoint(true); got != "https://fal.run/fal-ai/nano-banana/edit" {
		t.Fatalf("edit 엔드포인트 오류: %s", got)
	}
	// 이미 /edit이 붙은 모델은 중복 추가하지 않음
	c.Model = "fal-ai/nano-banana/edit"
	if got := c.endpoint(true); got != "https://fal.run/fal-ai/nano-banana/edit" {
		t.Fatalf("edit 중복 방지 실패: %s", got)
	}
}

func TestDecodeDataOrDownload(t *testing.T) {
	payload := []byte("hello-png-bytes")

	// data: URL 디코딩
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(payload)
	got, err := decodeDataOrDownload(http.DefaultClient, dataURL)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("data URL 디코딩 실패: %v %q", err, got)
	}

	// http URL 다운로드
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	got, err = decodeDataOrDownload(srv.Client(), srv.URL)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("다운로드 실패: %v %q", err, got)
	}

	// 잘못된 data URL
	if _, err := decodeDataOrDownload(http.DefaultClient, "data:image/png;hex,00"); err == nil {
		t.Fatal("base64 누락 data URL은 오류여야 합니다")
	}
}

func TestAspectHint(t *testing.T) {
	if h := aspectHint("1:1"); h != "Render on a square 1:1 canvas." {
		t.Fatalf("1:1 힌트 오류: %s", h)
	}
	if h := aspectHint("21:9"); h == "" || !contains(h, "21:9") {
		t.Fatalf("와이드 힌트 오류: %s", h)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
