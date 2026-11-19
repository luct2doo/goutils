package rsa

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

func TestEncryptWithPKCS1PublicKeyPEM(t *testing.T) {
	h := NewRSAHelper()
	_, privPEM, err := h.GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	priv, err := parsePrivateKey(privPEM)
	if err != nil {
		t.Fatalf("parsePrivateKey: %v", err)
	}

	// openssl rsa -RSAPublicKey_out 生成的 "RSA PUBLIC KEY"
	pkcs1PEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&priv.PublicKey),
	}))

	cipher, err := h.EncryptWithPublicKeyPKCS1v15("hello pkcs1", pkcs1PEM)
	if err != nil {
		t.Fatalf("EncryptWithPublicKeyPKCS1v15(PKCS#1): %v", err)
	}
	plain, err := h.DecryptWithPrivateKeyPKCS1v15(cipher, privPEM)
	if err != nil {
		t.Fatalf("DecryptWithPrivateKeyPKCS1v15: %v", err)
	}
	if plain != "hello pkcs1" {
		t.Errorf("plain = %q", plain)
	}
}

func TestEncryptWithPublicKeyTooLong(t *testing.T) {
	pubPEM, _, err := NewRSAHelper().GenerateKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	_, err = NewRSAHelper().EncryptWithPublicKey(strings.Repeat("a", 300), pubPEM)
	if err == nil || !strings.Contains(err.Error(), "明文太长") {
		t.Errorf("超长明文应返回友好错误，得到: %v", err)
	}
}

func TestParsePrivateKeyPKCS8(t *testing.T) {
	// 保留现有 PKCS#8 私钥兼容性
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if _, err := parsePrivateKey(pemStr); err != nil {
		t.Errorf("PKCS#8 私钥解析失败: %v", err)
	}
}
