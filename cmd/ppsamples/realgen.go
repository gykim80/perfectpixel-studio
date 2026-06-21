package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"time"

	"perfectpixel/internal/config"
	"perfectpixel/internal/gen"
	"perfectpixel/internal/sprite"
)

// 실제 OpenRouter(gemini-3-pro-image) 파이프라인으로 진짜 스프라이트를 생성한다.
// 합성 그림이 아니라 AI가 그린 마젠타 key 스트립을 비교 입력으로 쓴다.

const (
	demoDesc  = "a brave knight in blue steel armor with a flowing red cape and a round shield"
	demoState = "walk"
	demoN     = 6
)

func provider() (gen.Provider, string, error) {
	s := config.Load()
	cfg := s.Cfg(s.Provider)
	if cfg.APIKey == "" {
		return nil, "", fmt.Errorf("%s API 키 없음", s.Provider)
	}
	p, err := gen.New(s.Provider, cfg.APIKey, cfg.Model)
	return p, s.Provider, err
}

func decodePNGorJPEG(raw []byte) (*image.NRGBA, error) {
	im, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return sprite.ToNRGBA(im), nil
}

// buildRealBase는 베이스 캐릭터를 실제 생성해 마젠타 RGBA로 반환한다.
// (매팅·픽셀화 데모 입력. 분할·정렬 데모는 sample/의 실제 매팅 스트립을 쓴다.)
func buildRealBase() (*image.NRGBA, error) {
	// 이미 생성한 실제 베이스가 있으면 재사용(API 호출 절약, 동일 실제 산출물).
	if im, err := loadPNG(outDir + "/real-base.png"); err == nil {
		fmt.Println("  기존 real-base.png 재사용")
		return im, nil
	}
	p, name, err := provider()
	if err != nil {
		return nil, err
	}
	fmt.Printf("  provider=%s 베이스 캐릭터 실제 생성...\n", name)
	style := sprite.ResolveStyle("pixel", "")

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	bp := sprite.BuildCharacterPrompt(demoDesc, style, 256)
	t0 := time.Now()
	braw, err := p.GenerateImage(ctx, bp, nil, "1:1")
	if err != nil {
		return nil, fmt.Errorf("base 생성 실패: %w", err)
	}
	base, err := decodePNGorJPEG(braw)
	if err != nil {
		return nil, fmt.Errorf("base 디코딩 실패: %w", err)
	}
	fmt.Printf("  base 생성 완료 %dx%d (%.0fs)\n", base.Rect.Dx(), base.Rect.Dy(), time.Since(t0).Seconds())
	save("real-base.png", base)
	return base, nil
}

var _ = os.Stdout
var _ = demoState
