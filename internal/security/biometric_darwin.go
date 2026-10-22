package security

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"golang.org/x/crypto/pbkdf2"
)

type SystemBiometric struct{}

var loadAuthentication = sync.OnceValue(func() error {
	_, err := purego.Dlopen("/System/Library/Frameworks/LocalAuthentication.framework/LocalAuthentication", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	return err
})

func authContext() (objc.ID, error) {
	if err := loadAuthentication(); err != nil {
		return 0, err
	}
	return objc.ID(objc.GetClass("LAContext")).Send(objc.RegisterName("new")), nil
}

func (SystemBiometric) Available(context.Context) (bool, string) {
	object, err := authContext()
	if err != nil {
		return false, "无法加载 Touch ID，请使用软件密码"
	}
	defer object.Send(objc.RegisterName("release"))
	var nativeErr objc.ID
	if !objc.Send[bool](object, objc.RegisterName("canEvaluatePolicy:error:"), int64(1), &nativeErr) {
		return false, "此 Mac 未配置 Touch ID，请使用软件密码"
	}
	return true, "Touch ID"
}

func (SystemBiometric) Verify(ctx context.Context) error {
	object, err := authContext()
	if err != nil {
		return err
	}
	defer object.Send(objc.RegisterName("release"))
	done := make(chan bool, 1)
	block := objc.NewBlock(func(_ objc.Block, success bool, _ objc.ID) { done <- success })
	defer block.Release()
	reason := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithUTF8String:"), "解锁 Gist管理器")
	defer reason.Send(objc.RegisterName("release"))
	object.Send(objc.RegisterName("evaluatePolicy:localizedReason:reply:"), int64(1), reason, block)
	select {
	case success := <-done:
		if !success {
			return errors.New("Touch ID 已取消或验证失败")
		}
		return nil
	case <-ctx.Done():
		object.Send(objc.RegisterName("invalidate"))
		return ctx.Err()
	}
}

func decryptLegacyToken(value, _ string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(data) < 19 || string(data[:3]) != "v10" {
		return "", errors.New("无效的旧版凭据")
	}
	for _, name := range []string{"Gist管理器", "gist-editor-workspace"} {
		password, err := mygo.Secrets.Service(name + " Safe Storage").Get(name)
		if err != nil {
			continue
		}
		key := pbkdf2.Key(password, []byte("saltysalt"), 1003, 16, sha1.New)
		block, _ := aes.NewCipher(key)
		payload := data[3:]
		if len(payload)%aes.BlockSize != 0 {
			return "", errors.New("旧版凭据长度不正确")
		}
		plain := make([]byte, len(payload))
		cipher.NewCBCDecrypter(block, []byte("                ")).CryptBlocks(plain, payload)
		padding := int(plain[len(plain)-1])
		if padding < 1 || padding > aes.BlockSize || padding > len(plain) {
			continue
		}
		valid := true
		for _, b := range plain[len(plain)-padding:] {
			valid = valid && int(b) == padding
		}
		if valid {
			return string(plain[:len(plain)-padding]), nil
		}
	}
	return "", fmt.Errorf("无法读取旧版安全存储，请重新输入 Token")
}
