package main

import (
	"context"
	"log"
	"os"

	"github.com/muidea/workorch/application/workorchd/demo"
	"github.com/muidea/workorch/application/workorchd/presenter"
)

func main() {
	ctx := context.Background()
	slices, err := demo.RunAllFromEnv(ctx)
	if err != nil {
		log.Fatal(err)
	}

	presenter.PrintBanner(os.Stdout)
	for _, item := range slices {
		if err := presenter.PrintSnapshot(os.Stdout, item.Title, item.Snapshot); err != nil {
			log.Fatal(err)
		}
	}
}
