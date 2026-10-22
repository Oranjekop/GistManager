package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

type SystemBiometric struct{}

func (SystemBiometric) Available(ctx context.Context) (bool, string) {
	result, err := runHello(ctx, false, 0)
	if result != 0 || err != nil {
		return false, "此设备未配置 Windows Hello / PIN，请使用软件密码"
	}
	return true, "Windows Hello / PIN"
}

func (SystemBiometric) Verify(ctx context.Context) error {
	return (SystemBiometric{}).VerifyForWindow(ctx, 0)
}

func (SystemBiometric) VerifyForWindow(ctx context.Context, owner uintptr) error {
	result, err := runHello(ctx, true, owner)
	if err == nil && result != 0 {
		return errors.New("Windows Hello 已取消或验证失败")
	}
	return err
}

type dataBlob struct {
	Size uint32
	Data *byte
}

func decryptLegacyToken(value, directory string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(data) == 0 {
		return "", errors.New("无效的旧版凭据")
	}
	if len(data) > 3 && string(data[:3]) == "v10" {
		stateData, readErr := os.ReadFile(filepath.Join(directory, "Local State"))
		if readErr != nil {
			return "", readErr
		}
		var state struct {
			Crypt struct {
				Key string `json:"encrypted_key"`
			} `json:"os_crypt"`
		}
		if err = json.Unmarshal(stateData, &state); err != nil {
			return "", err
		}
		encryptedKey, err := base64.StdEncoding.DecodeString(state.Crypt.Key)
		if err != nil || len(encryptedKey) < 6 || string(encryptedKey[:5]) != "DPAPI" {
			return "", errors.New("旧版加密密钥格式不支持")
		}
		key, err := unprotect(encryptedKey[5:])
		if err != nil {
			return "", err
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return "", err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return "", err
		}
		if len(data) < 3+gcm.NonceSize()+gcm.Overhead() {
			return "", errors.New("旧版凭据不完整")
		}
		plain, err := gcm.Open(nil, data[3:3+gcm.NonceSize()], data[3+gcm.NonceSize():], nil)
		return string(plain), err
	}
	plain, err := unprotect(data)
	return string(plain), err
}

func unprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("凭据为空")
	}
	input := dataBlob{Size: uint32(len(data)), Data: &data[0]}
	var output dataBlob
	crypt := syscall.NewLazyDLL("crypt32.dll").NewProc("CryptUnprotectData")
	ok, _, err := crypt.Call(uintptr(unsafe.Pointer(&input)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&output)))
	if ok == 0 {
		return nil, err
	}
	defer syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree").Call(uintptr(unsafe.Pointer(output.Data)))
	return append([]byte(nil), unsafe.Slice(output.Data, int(output.Size))...), nil
}
