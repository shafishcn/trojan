package control

import "errors"

func validateBackupBundle(bundle BackupBundle) error {
	if len(bundle.Admins) == 0 {
		return errors.Join(ErrInvalidBackup, errors.New("backup must include at least one admin"))
	}
	for _, admin := range bundle.Admins {
		if admin.Role == "super_admin" && admin.Status == "active" {
			return nil
		}
	}
	return errors.Join(ErrInvalidBackup, errors.New("backup must include at least one active super_admin"))
}
