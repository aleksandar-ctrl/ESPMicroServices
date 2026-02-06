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

type DeviceStats struct {
	MAC string
	Avg string
	Min string
	Max string
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:pass@db:5432/mojabaza?sslmode=disable"
	}

	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Greska pri povezivanju sa bazom: %v\n", err)
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
		// zadnjih 10 logova
		rows, _ := conn.Query(context.Background(), "SELECT temperatura, COALESCE(device_id, 'Nepoznato'), TO_CHAR(vreme, 'HH24:MI:SS') FROM logovi ORDER BY id DESC LIMIT 10")
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

		// izvestaj za svaki uredjaj danas
		rowsStats, _ := conn.Query(context.Background(), `
			SELECT 
				device_id,
				COALESCE(ROUND(AVG(NULLIF(regexp_replace(temperatura, '[^0-9.]', '', 'g'), '')::numeric), 2)::text, '--'),
				COALESCE(MIN(temperatura), '--'),
				COALESCE(MAX(temperatura), '--')
			FROM logovi 
			WHERE vreme >= CURRENT_DATE 
			  AND temperatura != 'Komanda' 
			  AND device_id IS NOT NULL 
			  AND device_id != ''
			GROUP BY device_id`)

		var stList []DeviceStats
		for rowsStats.Next() {
			var s DeviceStats
			rowsStats.Scan(&s.MAC, &s.Avg, &s.Min, &s.Max)
			stList = append(stList, s)
		}

		tmpl := `
		<!DOCTYPE html>
		<html>
		<head>
			<title>IoT Dashboard</title>
			<meta name="viewport" content="width=device-width, initial-scale=1">
			<style>
				body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; text-align: center; background: #f0f2f5; padding: 10px; color: #333; }
				.main-temp { font-size: 65px; font-weight: bold; color: #2c3e50; margin: 10px 0; text-shadow: 1px 1px 2px rgba(0,0,0,0.1); }
				.container { display: flex; flex-wrap: wrap; justify-content: center; gap: 20px; margin-top: 20px; }
				.box { background: white; padding: 20px; border-radius: 12px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); flex: 1; min-width: 320px; max-width: 550px; }
				.btn { padding: 12px 20px; margin: 5px; text-decoration: none; display: inline-block; border-radius: 8px; color: black; border: 1px solid #ccc; font-weight: bold; transition: 0.3s; }
				.btn:hover { opacity: 0.8; transform: translateY(-2px); }
				.btn-white { background: #fff; } .btn-green { background: #2ecc71; color: white; border: none; }
				.btn-red { background: #e74c3c; color: white; border: none; } .btn-off { background: #34495e; color: white; border: none; }
				table { width: 100%; border-collapse: collapse; margin-top: 15px; background: #fff; }
				th, td { border-bottom: 1px solid #eee; padding: 12px; text-align: left; }
				th { background: #f8f9fa; color: #666; font-size: 12px; text-transform: uppercase; }
				.stat-val { font-size: 18px; font-weight: bold; color: #3498db; }
				.mac-label { font-family: monospace; color: #7f8c8d; font-size: 12px; }
			</style>
			<script>
				function update() {
        			fetch("/").then(r => r.text()).then(html => {
			            let doc = new DOMParser().parseFromString(html, 'text/html');
			            document.querySelector('.container').innerHTML = doc.querySelector('.container').innerHTML;
            			document.querySelector('.main-temp').innerHTML = doc.querySelector('.main-temp').innerHTML;
        			});
				}
				setInterval(update, 5000);
			</script>
		</head>
		<body>
			<p style="margin-bottom:0; color: #7f8c8d;">Zadnje očitavanje:</p>
			<div class="main-temp">{{.Zadnja}}</div>
			
			<div class="controls">
				<a href="/control?color=Bela" class="btn btn-white">BELA</a>
				<a href="/control?color=Zelena" class="btn btn-green">ZELENA</a>
				<a href="/control?color=Crvena" class="btn btn-red">CRVENA</a>
				<a href="/control?color=Off" class="btn btn-off">OFF</a>
			</div>

			<div class="container">
				<div class="box">
					<h3 style="margin-top:0;">Dnevni prosek po uređaju</h3>
					<table>
						<thead>
							<tr><th>Uređaj (MAC)</th><th>Prosek</th><th>Min / Max</th></tr>
						</thead>
						<tbody>
							{{range .StList}}
							<tr>
								<td class="mac-label">{{.MAC}}</td>
								<td class="stat-val">{{.Avg}}°C</td>
								<td style="font-size: 12px;">{{.Min}} / {{.Max}}</td>
							</tr>
							{{end}}
						</tbody>
					</table>
				</div>

				<div class="box">
					<h3 style="margin-top:0;">Poslednjih 10 zapisa</h3>
					<table>
						<thead>
							<tr><th>Uređaj</th><th>Vrednost</th><th>Vreme</th></tr>
						</thead>
						<tbody>
							{{range .Logs}}
							<tr>
								<td class="mac-label">{{.DeviceID}}</td>
								<td><strong>{{.Temp}}</strong></td>
								<td style="color: #95a5a6;">{{.Vreme}}</td>
							</tr>
							{{end}}
						</tbody>
					</table>
				</div>
			</div>
		</body>
		</html>`

		t := template.Must(template.New("w").Parse(tmpl))
		t.Execute(w, struct {
			Logs   []Log
			Zadnja string
			StList []DeviceStats
		}{logs, zadnjaTemp, stList})
	})

	fmt.Println("Server pokrenut na portu 8080...")
	http.ListenAndServe(":8080", nil)
}
