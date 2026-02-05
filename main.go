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
	connStr := os.Getenv("DB_URL")
	if connStr == "" {
		connStr = "postgres://user:pass@localhost:5432/mojabaza"
	}

	var conn *pgx.Conn
	var err error
	for i := 0; i < 10; i++ {
		conn, err = pgx.Connect(context.Background(), connStr)
		if err == nil {
			break
		}
		time.Sleep(3 * time.Second)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Baza nedostupna: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	// Pravimo tabelu
	conn.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS logovi (
		id SERIAL PRIMARY KEY, 
		temperatura TEXT, 
		boja TEXT, 
		vreme TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)

	// GLAVNA STRANICA - ISPISUJE I PODATKE
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8") // Popravlja kuke i motike

		// Čitamo poslednjih 10 zapisa iz baze
		rows, _ := conn.Query(context.Background(), "SELECT temperatura, boja, vreme FROM logovi ORDER BY vreme DESC LIMIT 10")
		defer rows.Close()

		tabela := "<table border='1' style='margin: 20px auto;'><tr><th>Temp</th><th>Boja</th><th>Vreme</th></tr>"
		for rows.Next() {
			var t, b string
			var v time.Time
			rows.Scan(&t, &b, &v)
			tabela += fmt.Sprintf("<tr><td>%s°C</td><td>%s</td><td>%s</td></tr>", t, b, v.Format("15:04:05"))
		}
		tabela += "</table>"

		fmt.Fprintf(w, `
			<div style="text-align: center; font-family: Arial;">
				<h1>🌡️ Trenutna temperatura: 25°C</h1>
				<a href="/promeni?boja=Zelena"><button>Zelena</button></a>
				<a href="/promeni?boja=Crvena"><button>Crvena</button></a>
				<hr>
				<h3>Poslednja očitavanja:</h3>
				%s
			</div>
		`, tabela)
	})

	// ENDPOINT ZA ESP
	http.HandleFunc("/esp", func(w http.ResponseWriter, r *http.Request) {
		temp := r.URL.Query().Get("temp")
		conn.Exec(context.Background(), "INSERT INTO logovi (temperatura, boja) VALUES ($1, $2)", temp, "ESP32_DATA")
		fmt.Println("ESP poslao:", temp)
		w.WriteHeader(200)
	})

	// ENDPOINT ZA DUGMAD
	http.HandleFunc("/promeni", func(w http.ResponseWriter, r *http.Request) {
		boja := r.URL.Query().Get("boja")
		conn.Exec(context.Background(), "INSERT INTO logovi (temperatura, boja) VALUES ($1, $2)", "25", boja)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	fmt.Println("🚀 Server spreman!")
	http.ListenAndServe(":8080", nil)
}
