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
	Temp  string
	Boja  string
	Vreme string
}

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:pass@db:5432/mojabaza?sslmode=disable"
	}

	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	conn.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS logovi (
		id SERIAL PRIMARY KEY, 
		temperatura TEXT, 
		boja TEXT, 
		vreme TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)

	http.HandleFunc("/esp", func(w http.ResponseWriter, r *http.Request) {
		temp := r.URL.Query().Get("temp")
		if temp != "" {
			tempFormat := temp + "C"
			conn.Exec(context.Background(), "INSERT INTO logovi (temperatura, boja) VALUES ($1, $2)", tempFormat, "ESP32_DATA")
		}

		var zadnjaBoja string
		err := conn.QueryRow(context.Background(), "SELECT boja FROM logovi WHERE temperatura = 'Komanda' ORDER BY id DESC LIMIT 1").Scan(&zadnjaBoja)
		if err == nil {
			fmt.Fprint(w, zadnjaBoja)
		} else {
			fmt.Fprint(w, "None")
		}
	})

	http.HandleFunc("/control", func(w http.ResponseWriter, r *http.Request) {
		boja := r.URL.Query().Get("color")
		if boja != "" {
			conn.Exec(context.Background(), "INSERT INTO logovi (temperatura, boja) VALUES ($1, $2)", "Komanda", boja)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rows, _ := conn.Query(context.Background(), "SELECT temperatura, boja, TO_CHAR(vreme, 'HH24:MI:SS') FROM logovi ORDER BY id DESC LIMIT 10")
		var logs []Log
		zadnjaTemp := "--"

		for rows.Next() {
			var l Log
			rows.Scan(&l.Temp, &l.Boja, &l.Vreme)
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

		tmpl := `
		<!DOCTYPE html>
		<html>
		<head>
			<title>ESP32 Dashboard</title>
			<meta http-equiv="refresh" content="5">
			<style>
				body { font-family: Arial; text-align: center; background-color: #f4f4f4; }
				.main-temp { font-size: 80px; font-weight: bold; margin: 40px 0; color: #333; }
				.btn { padding: 15px 25px; font-size: 18px; cursor: pointer; margin: 5px; border-radius: 5px; border: 1px solid #ccc; }
				.btn-white { background: white; }
				.btn-green { background: #4CAF50; color: white; }
				.btn-red { background: #f44336; color: white; }
				.btn-off { background: #333; color: white; }
				table { margin: 30px auto; border-collapse: collapse; width: 50%; background: white; }
				th, td { border: 1px solid #ddd; padding: 12px; }
				th { background-color: #eee; }
			</style>
			<script>
    			function update() {
		    	    fetch('/')
        	    		.then(response => response.text())
            			.then(html => {
                			const parser = new DOMParser();
			                const doc = parser.parseFromString(html, 'text/html');
                
			                const newTemp = doc.querySelector('.main-temp');
            			    const newTable = doc.querySelector('table');
                
                			if (newTemp && newTemp.innerHTML.trim() !== "") {
			                    document.querySelector('.main-temp').innerHTML = newTemp.innerHTML;
            			    }
                			if (newTable) {
			                    document.querySelector('table').innerHTML = newTable.innerHTML;
            			    }
            			})
           		 .catch(err => console.error('Greška:', err));
    			}	
			setInterval(update, 3000);
		</script>
		</head>
		<body>
			<h1>Trenutna temperatura:</h1>
			<div class="main-temp">{{.Zadnja}}</div>
			<div>
				<a href="/control?color=Bela"><button class="btn btn-white">Bela</button></a>
				<a href="/control?color=Zelena"><button class="btn btn-green">Zelena</button></a>
				<a href="/control?color=Crvena"><button class="btn btn-red">Crvena</button></a>
				<a href="/control?color=Off"><button class="btn btn-off">Ugaseno</button></a>
			</div>
			<h3>Poslednja ocitavanja:</h3>
			<table>
				<tr><th>Temp</th><th>Izvor/Boja</th><th>Vreme</th></tr>
				{{range .Logs}}
				<tr><td>{{.Temp}}</td><td>{{.Boja}}</td><td>{{.Vreme}}</td></tr>
				{{end}}
			</table>
		</body>
		</html>`

		t := template.Must(template.New("web").Parse(tmpl))
		t.Execute(w, struct {
			Logs   []Log
			Zadnja string
		}{Logs: logs, Zadnja: zadnjaTemp})
	})

	http.ListenAndServe(":8080", nil)
}
