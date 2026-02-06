package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
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

var dbConn *pgx.Conn
var mqttClient mqtt.Client

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://user:pass@db:5432/mojabaza?sslmode=disable"
	}
	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		fmt.Printf("Baza nije dostupna: %v\n", err)
		os.Exit(1)
	}
	dbConn = conn
	defer dbConn.Close(context.Background())

	dbConn.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS logovi (
       id SERIAL PRIMARY KEY, 
       temperatura TEXT, 
       device_id TEXT, 
       vreme TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`)

	// 2. MQTT Setup
	brokerURL := os.Getenv("MQTT_BROKER")
	if brokerURL == "" {
		brokerURL = "tcp://mqtt:1883"
	}
	opts := mqtt.NewClientOptions().AddBroker(brokerURL)
	opts.SetClientID("go_backend_server")

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		tempVrednost := string(msg.Payload())
		fmt.Printf("MQTT stiglo na topic [%s]: %s\n", msg.Topic(), tempVrednost)

		if msg.Topic() == "home/sensor/temp" {
			dbConn.Exec(context.Background(),
				"INSERT INTO logovi (temperatura, device_id) VALUES ($1, $2)",
				tempVrednost+"°C", "ESP32_S3")
		}
	})

	mqttClient = mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		fmt.Printf("MQTT Error: %v\n", token.Error())
	}

	mqttClient.Subscribe("home/sensor/temp", 0, nil)

	http.HandleFunc("/control", func(w http.ResponseWriter, r *http.Request) {
		boja := r.URL.Query().Get("color")
		if boja != "" {
			mqttClient.Publish("home/led/control", 0, false, boja)

			// Beležimo komandu u bazu
			dbConn.Exec(context.Background(), "INSERT INTO logovi (temperatura, device_id) VALUES ($1, $2)", "Komanda", boja)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rows, _ := dbConn.Query(context.Background(), "SELECT temperatura, COALESCE(device_id, 'Nepoznato'), TO_CHAR(vreme, 'HH24:MI:SS') FROM logovi ORDER BY id DESC LIMIT 10")
		var logs []Log
		zadnjaTemp := "--"
		for rows.Next() {
			var l Log
			rows.Scan(&l.Temp, &l.DeviceID, &l.Vreme)
			logs = append(logs, l)
		}

		if len(logs) > 0 {
			for _, l := range logs {
				if l.Temp != "Komanda" && l.Temp != "red" && l.Temp != "green" && l.Temp != "off" {
					zadnjaTemp = l.Temp
					break
				}
			}
		}

		rowsStats, _ := dbConn.Query(context.Background(), `
          SELECT 
             device_id,
             COALESCE(ROUND(AVG(NULLIF(regexp_replace(temperatura, '[^0-9.]', '', 'g'), '')::numeric), 2)::text, '--'),
             COALESCE(MIN(temperatura), '--'),
             COALESCE(MAX(temperatura), '--')
          FROM logovi 
          WHERE vreme >= CURRENT_DATE 
            AND temperatura NOT IN ('Komanda', 'red', 'green', 'off')
          GROUP BY device_id`)

		var stList []DeviceStats
		for rowsStats.Next() {
			var s DeviceStats
			rowsStats.Scan(&s.MAC, &s.Avg, &s.Min, &s.Max)
			stList = append(stList, s)
		}

		tmplCode := `
       <!DOCTYPE html>
       <html>
       <head>
          <title>MQTT IoT Dashboard</title>
          <meta name="viewport" content="width=device-width, initial-scale=1">
          <style>
             body { font-family: sans-serif; text-align: center; background: #f4f4f4; padding: 20px; }
             .main-temp { font-size: 60px; font-weight: bold; color: #2c3e50; margin: 20px 0; }
             .box { background: white; padding: 20px; border-radius: 10px; box-shadow: 0 2px 5px rgba(0,0,0,0.1); margin: 10px auto; max-width: 600px; }
             .btn { padding: 15px 25px; margin: 5px; border-radius: 5px; text-decoration: none; color: white; font-weight: bold; display: inline-block; border: none; cursor: pointer; }
             .btn-red { background: #e74c3c; } .btn-green { background: #2ecc71; } .btn-off { background: #34495e; }
             table { width: 100%; margin-top: 20px; border-collapse: collapse; }
             th, td { padding: 10px; border-bottom: 1px solid #ddd; text-align: left; }
             h2 { color: #7f8c8d; font-size: 18px; text-transform: uppercase; }
          </style>
          <script>
             function update() {
                fetch("/").then(r => r.text()).then(html => {
                   let doc = new DOMParser().parseFromString(html, 'text/html');
                   document.querySelector('.main-temp').innerHTML = doc.querySelector('.main-temp').innerHTML;
                   document.querySelectorAll('.box')[1].innerHTML = doc.querySelectorAll('.box')[1].innerHTML;
                   document.querySelectorAll('.box')[2].innerHTML = doc.querySelectorAll('.box')[2].innerHTML;
                });
             }
             setInterval(update, 3000);
          </script>
       </head>
       <body>
          <h1>IoT Control Center</h1>
          <h2>Trenutna Temperatura</h2>
          <div class="main-temp">{{.Zadnja}}</div>
          
          <div class="box">
             <h3>Kontrola LED Diode</h3>
             <a href="/control?color=red" class="btn btn-red">CRVENA</a>
             <a href="/control?color=green" class="btn btn-green">ZELENA</a>
             <a href="/control?color=off" class="btn btn-off">ISKLJUČI</a>
          </div>

          <div class="box">
             <h3>Dnevna Statistika</h3>
             <table>
                <tr><th>Uređaj</th><th>Prosek</th><th>Min/Max</th></tr>
                {{range .StList}}
                <tr><td>{{.MAC}}</td><td>{{.Avg}}°C</td><td>{{.Min}}/{{.Max}}</td></tr>
                {{end}}
             </table>
          </div>

          <div class="box">
             <h3>Poslednji zapisi</h3>
             <table>
                <tr><th>Uređaj</th><th>Vrednost</th><th>Vreme</th></tr>
                {{range .Logs}}
                <tr><td>{{.DeviceID}}</td><td>{{.Temp}}</td><td>{{.Vreme}}</td></tr>
                {{end}}
             </table>
          </div>
       </body>
       </html>`

		t := template.Must(template.New("web").Parse(tmplCode))
		t.Execute(w, struct {
			Logs   []Log
			Zadnja string
			StList []DeviceStats
		}{logs, zadnjaTemp, stList})
	})

	fmt.Println("Server pokrenut na portu 8080 (MQTT MOD)...")
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	server.ListenAndServe()
}
