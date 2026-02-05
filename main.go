package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	// 1. UZIMAMO ADRESU IZ DOCKER-COMPOSE ENVIRONMENTA
	connStr := os.Getenv("DB_URL")
	if connStr == "" {
		// Rezervna opcija ako pokrećeš van Dockera
		connStr = "postgres://user:pass@localhost:5432/mojabaza"
	}

	var conn *pgx.Conn
	var err error

	// 2. RETRY LOGIKA (Čekamo da se baza probudi)
	fmt.Println("Povezivanje na bazu...")
	for i := 0; i < 10; i++ {
		conn, err = pgx.Connect(context.Background(), connStr)
		if err == nil {
			break
		}
		fmt.Printf("Baza još nije spremna, pokušaj %d/10...\n", i+1)
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Greška: Baza nije dostupna: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	// 3. KREIRANJE TABELE (Automatski pri startu)
	_, err = conn.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS logovi (
			id SERIAL PRIMARY KEY,
			temperatura INT,
			boja TEXT,
			vreme TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		fmt.Println("Greška pri kreiranju tabele:", err)
	}

	// 4. FRONTEND (Glavna stranica sa temperaturom i dugmićima)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		temp := 25 // Simulirana temperatura

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
			<div style="text-align: center; font-family: Arial; margin-top: 100px;">
				<h1>🌡️ Trenutna temperatura: %d°C</h1>
				<h3>Izaberi akciju:</h3>
				<a href="/promeni?boja=Bela"><button style="background: white; padding: 10px;">Bela</button></a>
				<a href="/promeni?boja=Zelena"><button style="background: green; color: white; padding: 10px;">Zelena</button></a>
				<a href="/promeni?boja=Crvena"><button style="background: red; color: white; padding: 10px;">Crvena</button></a>
				<a href="/promeni?boja=Ugaseno"><button style="background: black; color: white; padding: 10px;">Ugaseno</button></a>
			</div>
		`, temp)
	})

	// 5. LOGIKA UPISA (Kada klikneš dugme)
	http.HandleFunc("/promeni", func(w http.ResponseWriter, r *http.Request) {
		boja := r.URL.Query().Get("boja")
		temp := 25

		_, err := conn.Exec(context.Background(),
			"INSERT INTO logovi (temperatura, boja) VALUES ($1, $2)", temp, boja)

		if err != nil {
			http.Error(w, "Greška pri upisu u bazu", 500)
			return
		}

		fmt.Fprintf(w, "Upisano: Boja %s, Temp %d. <br><a href='/'>Nazad</a>", boja, temp)
	})

	fmt.Println("🚀 Server startovan na http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}
