package kubernetes

import (
	"k8s.io/apimachinery/pkg/api/errors"
)

func IsNotFound(err error) bool {
	return errors.IsNotFound(err)
}

func IsAlreadyExists(err error) bool {
	return errors.IsAlreadyExists(err)
}

func IsConflict(err error) bool {
	return errors.IsConflict(err)
}
