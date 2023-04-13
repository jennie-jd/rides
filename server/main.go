package main

import (
	db "app/db"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"os"
	"path/filepath"
	"time"
	"fmt"

	"github.com/joho/godotenv"
	"github.com/gorilla/mux"
)

type Driver struct {
	Id       string `json:"id"`
	DriverId string `json:"driverId"`
	Name        string `json:"name"`
	Status 		  string `json:"status"`
	Location string `json:"location"`
	Path     string `json:"path"`
	PathIndex int `json:"pathIndex"`
	CustomerId string `json:"customerId"`
}

type Customer struct {
	Id          string `json:"id"`
	CustomerId  string `json:"customerId"`
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	Location    string `json:"location"`
	Destination string `json:"destination"`
	DriverId    string `json:"driverId"`
}

func getDrivers(w http.ResponseWriter, req *http.Request) {
	rows, err := db.Connection.Query(`
		SELECT
			id,
			driver_id,
			name, 
			status, 
			location,
			path,
			path_index,
			customer_id 
		FROM drivers
		`,
	)
	if err != nil {
		http.Error(w, "Failed to get drivers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var drivers []Driver

	for rows.Next() {
		var driver Driver
		rows.Scan(
			&driver.Id,
			&driver.DriverId,
			&driver.Name,
			&driver.Status,
			&driver.Location,
			&driver.Path,
			&driver.PathIndex,
			&driver.CustomerId,
		)
		drivers = append(drivers, driver)
	}

	ridesBytes, _ := json.MarshalIndent(drivers, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(ridesBytes)
}

func getCustomers(w http.ResponseWriter, req *http.Request) {
	rows, err := db.Connection.Query(`
		SELECT 
			id, 
			customer_id, 
			name, 
			active, 
			location, 
			destination, 
			driver_id 
		FROM customers WHERE
		active = true AND 
		driver_id IS NULL AND 
		(location IS NOT NULL AND location != 'null') 
		`,
	)
	if err != nil {
		http.Error(w, "Failed to get customers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var customers []Customer

	for rows.Next() {
		var customer Customer
		rows.Scan(
			&customer.Id,
			&customer.CustomerId,
			&customer.Name,
			&customer.Active,
			&customer.Location,
			&customer.Destination,
			&customer.DriverId,
		)
		customers = append(customers, customer)
	}

	ridesBytes, _ := json.MarshalIndent(customers, "", "\t")

	w.Header().Set("Content-Type", "application/json")
	w.Write(ridesBytes)
}

func getGrafanaProxy() *httputil.ReverseProxy {
	baseURL := "http://host.docker.internal"
	if os.Getenv("SERVER_ENV") == "PROD" {
		baseURL = "http://" + os.Getenv("SERVER_IP")
	}
	grafanaURL, _ := url.Parse(baseURL + ":3000")
	fmt.Println("Grafana URL: " + grafanaURL.String())
	grafanaProxy := httputil.NewSingleHostReverseProxy(grafanaURL)
	return grafanaProxy
}

func getPrometheusProxy() *httputil.ReverseProxy {
	baseURL := "http://host.docker.internal"
	if os.Getenv("SERVER_ENV") == "PROD" {
		baseURL = "http://" + os.Getenv("SERVER_IP")
	}
	prometheusURL, _ := url.Parse(baseURL + ":9090")
	fmt.Println("Prometheus URL: " + prometheusURL.String())
	prometheusProxy := httputil.NewSingleHostReverseProxy(prometheusURL)
	return prometheusProxy
}

type spaHandler struct {
	staticPath string
	indexPath  string
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	path = filepath.Join(h.staticPath, path)

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

func proxyHandler(p *httputil.ReverseProxy) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/proxy/grafana")
		p.ServeHTTP(w, r)
	}
}

func main() {
	err := godotenv.Load("../.env")
  if err != nil {
    log.Fatal(err)
  }

	db.InitDB()
	defer db.Connection.Close()

	router := mux.NewRouter()

	grafanaProxy := getGrafanaProxy()
	prometheusProxy := getPrometheusProxy()
	
	router.HandleFunc("/api/drivers", getDrivers)
	router.HandleFunc("/api/customers", getCustomers)

	router.HandleFunc("/proxy/grafana/", proxyHandler(grafanaProxy))
	// router.HandleFunc("/grafana/", func(w http.ResponseWriter, r *http.Request) {
	// 	// Modify the incoming request URL to remove the "/grafana" prefix.
  //   r.URL.Path = strings.TrimPrefix(r.URL.Path, "/grafana")
  //   grafanaProxy.ServeHTTP(w, r)
	// })

	router.HandleFunc("/prometheus/", func(w http.ResponseWriter, r *http.Request) {
		// Modify the incoming request URL to remove the "/grafana" prefix.
    // r.URL.Path = strings.TrimPrefix(r.URL.Path, "/prometheus")
		// don't overwrite if asking for /metrics!
    prometheusProxy.ServeHTTP(w, r)
	})

	spa := spaHandler{staticPath: "../frontend/build", indexPath: "index.html"}
	router.PathPrefix("/").Handler(spa)

	serverEnv := os.Getenv("SERVER_ENV")
	if serverEnv == "DEV" {
		srv := &http.Server{
			Handler: router,
			Addr:    ":8080",
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		err := srv.ListenAndServe()
		if err != nil {
				log.Fatal(err)
		}
		
	} else if serverEnv == "PROD" {
		srv := &http.Server{
			Handler: router,
			Addr:    ":443",
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}
	
		log.Fatal(srv.ListenAndServeTLS(
			"/etc/letsencrypt/live/rides.jurajmajerik.com/fullchain.pem",
			"/etc/letsencrypt/live/rides.jurajmajerik.com/privkey.pem",
		))
	}
}
