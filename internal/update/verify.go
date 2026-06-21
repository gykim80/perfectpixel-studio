package update

import (
	"errors"
	"fmt"

	"aead.dev/minisign"
)

// publicKey는 데스크톱 릴리스 산출물을 서명하는 minisign 공개 키입니다. 공개
// 절반은 임베드해도 안전하며, 비밀 절반은 CI 시크릿에만 존재합니다(cmd/sign
// genkey 로 생성). 서명 키를 교체(rotate)하면 이 상수와 CI 시크릿을 같이
// 갱신해야 합니다.
const publicKey = `untrusted comment: minisign public key: 5A90D4A2ED827B0D
RWQNe4LtotSQWpBQlVzBUKGwMUjmBEu5Z+iHEqMWCKLrL0HJKvfVxYPm`

// Verify는 sig(.minisig 파일 내용)가 임베드된 공개 키로 data 의 유효한 minisign
// 서명인지 보고합니다. nil 반환은 산출물이 진짜임을 의미하고, 어떤 에러든
// 신뢰하면 안 됨을 뜻합니다. 호출자는 디스크에 손대기 전에 반드시 검증해야 하며,
// 서명이 확인되지 않은 업데이트는 절대 적용하지 마십시오.
func Verify(data, sig []byte) error { return verifyWith(publicKey, data, sig) }

// PublicKey는 임베드된 공개 키를 표준 2줄 텍스트 형태로 반환합니다(docs/UI 가
// 수동 `minisign -Vm <file>` 검증을 위해 노출할 수 있도록).
func PublicKey() string { return publicKey }

// verifyWith는 테스트 가능한 코어입니다: 임의의 공개 키 텍스트를 파싱해 서명을
// 검증하므로, 테스트가 임베드된 키의 (비밀) 상대 없이 일회용 키 쌍을 쓸 수 있습니다.
func verifyWith(pubText string, data, sig []byte) error {
	var key minisign.PublicKey
	if err := key.UnmarshalText([]byte(pubText)); err != nil {
		return fmt.Errorf("update: parse public key: %w", err)
	}
	if !minisign.Verify(key, data, sig) {
		return errors.New("update: signature verification failed")
	}
	return nil
}
