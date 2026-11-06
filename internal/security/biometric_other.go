//go:build !windows && !darwin

package security

import (
	"context"

	"github.com/Oranjekop/GistManager/internal/l10n"
)

type SystemBiometric struct{}

func (SystemBiometric) Available(context.Context) (bool, string) {
	return false, l10n.Text("此系统暂不支持生物识别，请使用软件密码")
}

func (SystemBiometric) Verify(context.Context) error {
	return l10n.Error("此系统暂不支持生物识别")
}

func decryptLegacyToken(string, string) (string, error) {
	return "", l10n.Error("请重新输入 GitHub Token")
}
