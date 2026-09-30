//go:build !windows

package windowsclient

func SavePassword(server, username, password string) error {
	return ErrCredentialStoreUnsupported
}

func LoadPassword(server, username string) (string, error) {
	return "", ErrCredentialStoreUnsupported
}

func DeletePassword(server, username string) error {
	return ErrCredentialStoreUnsupported
}
