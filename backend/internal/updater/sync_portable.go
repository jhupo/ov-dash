//go:build !linux

package updater

func syncDirectory(string) error {
	return nil
}
