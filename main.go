package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	connStr := "postgres://user:pass@db:5432/mojabaza"
	var conn *pgx.Conn
	var err error

	// Pokušaj 10 puta, jednom svake sekunde
	for i := 0; i < 10; i++ {
		fmt.Printf("Pokušaj povezivanja na bazu... (%d/10)\n", i+1)
		conn, err = pgx.Connect(context.Background(), connStr)
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Baza nije dostupna nakon 10 pokušaja: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	fmt.Println("KONAČNO! Uspešno povezan na Postgres!")
}
