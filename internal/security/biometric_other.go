//go:build !windows && !darwin

package security

import (
	"context"
	"errors"
)

type SystemBiometric struct{}

func (SystemBiometric) Available(context.Context) (bool, string) {
	return false, "此系统暂不支持生物识别，请使用软件密码"
}

func (SystemBiometric) Verify(context.Context) error {
	return errors.New("此系统暂不支持生物识别")
}

func decryptLegacyToken(string, string) (string, error) {
	return "", errors.New("请重新输入 GitHub Token")
}
