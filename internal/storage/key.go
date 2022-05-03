package storage

import "hash/fnv"

func generateKey(url *string) (uint64, error) {
	hasher := fnv.New64()
	_, err := hasher.Write([]byte(*url))
	if err != nil {
		return 0, err
	}
	return hasher.Sum64(), nil
}
