package random

import (
	"crypto/rand"
	"github.com/myrjola/petrapp/internal/errors"
	"math/big"
)

func Letters(n uint) (string, error) {
	var allowedLetters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	letters := make([]rune, n)
	for i := range letters {
		letterIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", errors.Wrap(err, "generate random integer")
		}
		letters[i] = allowedLetters[letterIndex.Int64()]
	}
	return string(letters), nil
}
