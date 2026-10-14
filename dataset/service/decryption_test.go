package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"testing"
)

func encryptTestAES(plaintext, key string) string {
	h := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(h[:])
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "aes:" + base64.StdEncoding.EncodeToString(ciphertext)
}

func TestDatasetServiceAESDecryption(t *testing.T) {
	key := "test-secret-key-12345"
	svc := &DataSetService{}
	svc.SetDecryptionKey(key)

	plainReg := "KA-01-AB-9999"
	plainPhone := "+919876543210"
	plainSecret := "confidential-token-xyz"

	encReg := encryptTestAES(plainReg, key)
	encPhone := encryptTestAES(plainPhone, key)
	encSecret := encryptTestAES(plainSecret, key)

	rows := []map[string]any{
		{
			"id":              1,
			"registration_no": encReg,
			"driver": map[string]any{
				"name":  "John Doe",
				"phone": encPhone,
			},
			"childarr": []any{
				map[string]any{
					"item_id": 101,
					"secret":  encSecret,
				},
			},
		},
	}

	decryptedRows := svc.normalizeAndDecryptDatasetRows(context.Background(), rows)

	if len(decryptedRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(decryptedRows))
	}
	row := decryptedRows[0]

	// Verify top-level field
	if row["registration_no"] != plainReg {
		t.Errorf("registration_no expected %q, got %q", plainReg, row["registration_no"])
	}

	// Verify nested object field
	driver, ok := row["driver"].(map[string]any)
	if !ok {
		t.Fatalf("driver is not a map: %T", row["driver"])
	}
	if driver["phone"] != plainPhone {
		t.Errorf("driver.phone expected %q, got %q", plainPhone, driver["phone"])
	}

	// Verify array of objects field
	childarr, ok := row["childarr"].([]any)
	if !ok || len(childarr) == 0 {
		t.Fatalf("childarr missing or empty: %v", row["childarr"])
	}
	childItem, ok := childarr[0].(map[string]any)
	if !ok {
		t.Fatalf("childarr item is not a map: %T", childarr[0])
	}
	if childItem["secret"] != plainSecret {
		t.Errorf("childarr[0].secret expected %q, got %q", plainSecret, childItem["secret"])
	}
}

func TestDatasetServiceCustomDecryptFunc(t *testing.T) {
	svc := &DataSetService{}
	svc.SetDecryptionKey("dummy-key")
	svc.SetDecryptFunc(func(ctx context.Context, encrypted, key string) (string, error) {
		if encrypted == "enc:custom-pgp-test" {
			return "decrypted-via-custom-hook", nil
		}
		return encrypted, nil
	})

	rows := []map[string]any{
		{"data": "enc:custom-pgp-test"},
	}

	decryptedRows := svc.normalizeAndDecryptDatasetRows(context.Background(), rows)
	if decryptedRows[0]["data"] != "decrypted-via-custom-hook" {
		t.Errorf("expected decrypted-via-custom-hook, got %v", decryptedRows[0]["data"])
	}
}
