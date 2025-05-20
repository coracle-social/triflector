package main

import (
	"fmt"
	"log"

  "frith/common"
	"github.com/dgraph-io/badger/v4"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	common.SetupEnvironment()
	common.SetupDatabase()

	defer common.Db.Close()

	err := common.Db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()

			err := item.Value(func(val []byte) error {
				fmt.Printf("%s\t%s\n", key, string(val))
				return nil
			})

			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Fatal("Error reading database:", err)
	}
}
