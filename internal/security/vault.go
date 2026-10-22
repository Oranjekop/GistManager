package security

import (
	"errors"

	"github.com/egoist/mygo"
)

type SystemVault struct{ Service string }

func (v SystemVault) Load() (string, error) {
	value, err := mygo.Secrets.Service(v.Service).Get("github-token")
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return "", nil
	}
	return string(value), err
}

func (v SystemVault) Save(token string) error {
	return mygo.Secrets.Service(v.Service).Set("github-token", []byte(token))
}

func (v SystemVault) Delete() error { return mygo.Secrets.Service(v.Service).Delete("github-token") }
