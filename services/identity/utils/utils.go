package utils

import "golang.org/x/crypto/bcrypt"

func Bcrypt(password string) (string, error) {
	passwordHash := []byte(password)
	cost := 12

	hashedPassword, err := bcrypt.GenerateFromPassword(passwordHash, cost)
	if err != nil {
		return "", err
	}

	return string(hashedPassword), nil
}
