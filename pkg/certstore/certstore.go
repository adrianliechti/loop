package certstore

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"strings"
)

func certFingerprint(path string) (string, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		return "", err
	}

	block, _ := pem.Decode(data)

	if block == nil {
		return "", errors.New("no PEM block found")
	}

	cert, err := x509.ParseCertificate(block.Bytes)

	if err != nil {
		return "", err
	}

	hash := sha1.Sum(cert.Raw)

	return strings.ToUpper(hex.EncodeToString(hash[:])), nil
}
