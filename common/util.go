package common

import (
  "strings"
	"math/rand"
)

var AUTH_JOIN = 28934

var AUTH_INVITE = 28935

func keys[K comparable, V any](m map[K]V) []K {
	ks := make([]K, len(m))

	i := 0
	for k := range m {
		ks[i] = k
		i++
	}

	return ks
}

func filter[T any](ss []T, test func(T) bool) (ret []T) {
	for _, s := range ss {
		if test(s) {
			ret = append(ret, s)
		}
	}

	return
}

const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

func split(s string, delim string) []string {
  if s == "" {
    return []string{}
  } else {
  	return strings.Split(s, delim)
  }
}
