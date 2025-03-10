package system

import (
	"io"
	"os"
	"sync"
)

type AtomicFile struct {
	path string
	mu   sync.Mutex
}

func NewAtomicFile(path string) (*AtomicFile, error) {
	return &AtomicFile{
		path: path,
	}, nil
}

func (a *AtomicFile) ReadAll() ([]byte, error) {
	file, err := os.Open(a.path)

	if err != nil {
		if os.IsNotExist(err) {
			return []byte{}, nil
		}

		return nil, err
	}

	defer file.Close()

	return io.ReadAll(file)
}

func (a *AtomicFile) WriteString(text string) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	file, err := os.OpenFile(a.path, os.O_RDWR, 0644)

	if err != nil {
		return 0, err
	}

	defer file.Close()

	if _, err := file.Seek(0, 0); err != nil {
		return 0, err
	}

	if err := file.Truncate(0); err != nil {
		return 0, err
	}

	return file.WriteString(text)
}
