package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func main() {
	// "db" je ime servisa koje smo stavili u docker-compose.yml
	connStr := "postgres://user:pass@db:5432/mojabaza"

	conn, err := pgx.Connect(context.Background(), connStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Neuspešno povezivanje na bazu: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	fmt.Println("Uspešno povezan na Postgres unutar Dockera!")
}
