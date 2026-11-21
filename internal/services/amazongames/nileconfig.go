package amazongames

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // nile names the user-data file by md5(user_id); a file name, not a security use
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// nile's account files (nile/utils/config.py, v1.2.0):
//
//	<NILE_CONFIG_PATH>/nile/current_user.json  {"name": …, "user_id": …}
//	<NILE_CONFIG_PATH>/nile/<md5(user_id)>.enc
//
// The .enc file is the user data as JSON, AES-256-CBC with PKCS#7 padding
// under key sha256(user_id), preceded by the 16-byte IV encrypted with the
// same key in ECB mode.

type currentUser struct {
	Name   string `json:"name"`
	UserID string `json:"user_id"`
}

// accountOf reads the account identity out of nile's user data.
func accountOf(userData json.RawMessage) (*Account, error) {
	var u struct {
		Extensions struct {
			CustomerInfo struct {
				UserID    string `json:"user_id"`
				GivenName string `json:"given_name"`
			} `json:"customer_info"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(userData, &u); err != nil {
		return nil, errors.New("amazon: stored sign-in is not nile user data")
	}
	if u.Extensions.CustomerInfo.UserID == "" {
		return nil, errors.New("amazon: stored sign-in has no Amazon user id")
	}
	return &Account{UserID: u.Extensions.CustomerInfo.UserID, Name: u.Extensions.CustomerInfo.GivenName}, nil
}

func userDataFileName(userID string) string {
	sum := md5.Sum([]byte(userID)) //nolint:gosec // file name only, matching nile
	return hex.EncodeToString(sum[:]) + ".enc"
}

// writeNileConfig writes nile's account files for userData into dir.
func writeNileConfig(dir string, userData json.RawMessage) error {
	account, err := accountOf(userData)
	if err != nil {
		return err
	}
	nileDir := filepath.Join(dir, "nile")
	if err := os.MkdirAll(nileDir, 0o700); err != nil {
		return fmt.Errorf("amazon: write nile config: %w", err)
	}
	cu, err := json.Marshal(currentUser{Name: account.Name, UserID: account.UserID})
	if err != nil {
		return fmt.Errorf("amazon: write nile config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(nileDir, "current_user.json"), cu, 0o600); err != nil {
		return fmt.Errorf("amazon: write nile config: %w", err)
	}
	enc, err := encryptUserData(account.UserID, userData)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(nileDir, userDataFileName(account.UserID)), enc, 0o600); err != nil {
		return fmt.Errorf("amazon: write nile config: %w", err)
	}
	return nil
}

// readNileConfig reads and decrypts the user data nile left in dir.
func readNileConfig(dir string) (json.RawMessage, error) {
	nileDir := filepath.Join(dir, "nile")
	cuRaw, err := os.ReadFile(filepath.Join(nileDir, "current_user.json")) //nolint:gosec // path inside a temp config dir this package created
	if err != nil {
		return nil, errors.New("amazon: nile wrote no signed-in user")
	}
	var cu currentUser
	if err := json.Unmarshal(cuRaw, &cu); err != nil || cu.UserID == "" {
		return nil, errors.New("amazon: nile's current_user.json has no user id")
	}
	enc, err := os.ReadFile(filepath.Join(nileDir, userDataFileName(cu.UserID))) //nolint:gosec // path inside a temp config dir this package created
	if err != nil {
		return nil, errors.New("amazon: nile wrote no user data")
	}
	return decryptUserData(cu.UserID, enc)
}

func encryptUserData(userID string, userData json.RawMessage) ([]byte, error) {
	key := sha256.Sum256([]byte(userID))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("amazon: encrypt user data: %w", err)
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("amazon: encrypt user data: %w", err)
	}
	pad := aes.BlockSize - len(userData)%aes.BlockSize
	plain := append(append([]byte{}, userData...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, aes.BlockSize+len(plain))
	block.Encrypt(out[:aes.BlockSize], iv) // ECB of the single IV block
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out[aes.BlockSize:], plain)
	return out, nil
}

func decryptUserData(userID string, enc []byte) (json.RawMessage, error) {
	bad := errors.New("amazon: nile user data could not be decrypted")
	if len(enc) < 2*aes.BlockSize || len(enc)%aes.BlockSize != 0 {
		return nil, bad
	}
	key := sha256.Sum256([]byte(userID))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, bad
	}
	iv := make([]byte, aes.BlockSize)
	block.Decrypt(iv, enc[:aes.BlockSize])
	plain := make([]byte, len(enc)-aes.BlockSize)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, enc[aes.BlockSize:])
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || !bytes.Equal(plain[len(plain)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, bad
	}
	plain = plain[:len(plain)-pad]
	if !json.Valid(plain) {
		return nil, bad
	}
	return plain, nil
}
