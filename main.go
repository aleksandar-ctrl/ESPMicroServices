package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"
)

type Log struct {
	Temp     string
	DeviceID string
	Vreme    string
}

type Stats struct {
	Avg string
	Min string
	Max string
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://root:aca01234@db:5432/mojabaza?sslmode=disable"
	}

	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Greska: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	conn.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS logovi (
		id SERIAL PRIMARY KEY, 
		temperatura TEXT, 
		device_id TEXT, 
		vreme TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)

	http.HandleFunc("/esp", func(w http.ResponseWriter, r *http.Request) {
		temp := r.URL.Query().Get("temp")
		mac := r.URL.Query().Get("mac")
		if temp != "" {
			conn.Exec(context.Background(), "INSERT INTO logovi (temperatura, device_id) VALUES ($1, $2)", temp+"C", mac)
		}
		var zadnja string
		conn.QueryRow(context.Background(), "SELECT device_id FROM logovi WHERE temperatura = 'Komanda' ORDER BY id DESC LIMIT 1").Scan(&zadnja)
		fmt.Fprint(w, zadnja)
	})

	http.HandleFunc("/control", func(w http.ResponseWriter, r *http.Request) {
		boja := r.URL.Query().Get("color")
		if boja != "" {
			conn.Exec(context.Background(), "INSERT INTO logovi (temperatura, device_id) VALUES ($1, $2)", "Komanda", boja)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Zadnjih 10 logova
		rows, _ := conn.Query(context.Background(), "SELECT temperatura, device_id, TO_CHAR(vreme, 'HH24:MI:SS') FROM logovi ORDER BY id DESC LIMIT 10")
		var logs []Log
		zadnjaTemp := "--"
		for rows.Next() {
			var l Log
			rows.Scan(&l.Temp, &l.DeviceID, &l.Vreme)
			logs = append(logs, l)
		}
		if len(logs) > 0 {
			for _, l := range logs {
				if l.Temp != "Komanda" {
					zadnjaTemp = l.Temp
					break
				}
			}
		}

		var st Stats
		// statistika za danas
		conn.QueryRow(context.Background(), `
			SELECT 
				COALESCE(ROUND(AVG(NULLIF(regexp_replace(temperatura, '[^0-9.]', '', 'g'), '')::numeric), 2)::text, '--'),
				COALESCE(MIN(temperatura), '--'),
				COALESCE(MAX(temperatura), '--')
			FROM logovi WHERE vreme >= CURRENT_DATE AND temperatura != 'Komanda'`).Scan(&st.Avg, &st.Min, &st.Max)

		tmpl := `
		<!DOCTYPE html>
		<html>
		<head>
			<title>ESP32 Analitika</title>
			<meta name="viewport" content="width=device-width, initial-scale=1">
			<style>
				body { font-family: Arial; text-align: center; background: #f4f4f4; padding: 10px; }
				.main-temp { font-size: 60px; font-weight: bold; color: #333; margin: 10px; }
				.container { display: flex; flex-wrap: wrap; justify-content: center; gap: 20px; }
				.box { background: white; padding: 15px; border-radius: 10px; box-shadow: 0 2px 5px rgba(0,0,0,0.1); flex: 1; min-width: 300px; max-width: 500px; }
				.btn { padding: 10px 15px; margin: 5px; text-decoration: none; display: inline-block; border-radius: 5px; color: black; border: 1px solid #ccc; font-weight: bold; }
				.btn-white { background: white; } .btn-green { background: #4CAF50; color: white; }
				.btn-red { background: #f44336; color: white; } .btn-off { background: #333; color: white; }
				table { width: 100%; border-collapse: collapse; margin-top: 10px; font-size: 13px; }
				th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
				th { background: #eee; }
				.stat-val { font-size: 20px; font-weight: bold; color: #007bff; }
			</style>
			<script>
				function update() {
					fetch("/").then(r => r.text()).then(html => {
						let doc = new DOMParser().parseFromString(html, 'text/html');
						document.body.innerHTML = doc.body.innerHTML;
					});
				}
				setInterval(update, 5000);
			</script>
		</head>
		<body>
			<h2>Trenutna temperatura:</h2>
			<div class="main-temp">{{.Zadnja}}</div>
			<div>
				<a href="/control?color=Bela" class="btn btn-white">Bela</a>
				<a href="/control?color=Zelena" class="btn btn-green">Zelena</a>
				<a href="/control?color=Crvena" class="btn btn-red">Crvena</a>
				<a href="/control?color=Off" class="btn btn-off">Off</a>
			</div>
			<br>
			<div class="container">
				<div class="box">
					<h3>Dnevni Izvestaj</h3>
					<p>Prosek: <span class="stat-val">{{.St.Avg}}C</span></p>
					<p>Min: <span class="stat-val">{{.St.Min}}</span> | Max: <span class="stat-val">{{.St.Max}}</span></p>
				</div>
				<div class="box">
					<h3>Zadnjih 10 ocitavanja</h3>
					<table>
						<tr><th>Uredjaj (MAC)</th><th>Temp</th><th>Vreme</th></tr>
						{{range .Logs}}
						<tr><td>{{.DeviceID}}</td><td>{{.Temp}}</td><td>{{.Vreme}}</td></tr>
						{{end}}
					</table>
				</div>
			</div>
		</body>
		</html>`
		t := template.Must(template.New("w").Parse(tmpl))
		t.Execute(w, struct {
			Logs   []Log
			Zadnja string
			St     Stats
		}{logs, zadnjaTemp, st})
	})
	http.ListenAndServe(":8080", nil)
}
