package common

import (
	"fmt"
	"log"
	"github.com/dgraph-io/badger/v4"
)

var Db *badger.DB

func SetupDatabase() {
	var err error
	Db, err = badger.Open(badger.DefaultOptions(GetDataDir("frith")))
	if err != nil {
		log.Fatal("Failed to open badger db:", err)
	}
}

func PutItem(tbl string, key string, value string) {
	if err := Db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(tbl+":"+key), []byte(value))
	}); err != nil {
		fmt.Println(err)
	}
}

func GetItem(tbl string, key string) string {
	var result string
	err := Db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(tbl + ":" + key))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			result = string(val)
			return nil
		})
	})

	if err != nil && err != badger.ErrKeyNotFound {
		fmt.Println(err)
	}

	return result
}

func DeleteItem(tbl string, key string) {
	err := Db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(tbl + ":" + key))
	})
	if err != nil {
		fmt.Println(err)
	}
}
