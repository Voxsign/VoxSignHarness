package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
)

var (
	addr     = flag.String("addr", "127.0.0.1:8099", "HTTP network address")
	dataDir  = flag.String("data-dir", ".", "Data directory")
	userFile = "user_profile.json"
	mu       sync.Mutex
	userData = make(map[string]string)
)

func main() {
	flag.Parse()
	loadUserData()

	http.HandleFunc("/v1/set_name", setNameHandler)
	http.HandleFunc("/v1/get_name", getNameHandler)
	http.HandleFunc("/v1/health", healthHandler)

	fmt.Printf("Listening on %s...\n", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		fmt.Println("Server failed:", err)
	}
}

func loadUserData() {
	mu.Lock()
	defer mu.Unlock()

	filePath := fmt.Sprintf("%s/%s", *dataDir, userFile)
	data, err := ioutil.ReadFile(filePath)
	if err == nil {
		json.Unmarshal(data, &userData)
	}
}

func saveUserData() {
	mu.Lock()
	defer mu.Unlock()

	filePath := fmt.Sprintf("%s/%s", *dataDir, userFile)
	data, _ := json.Marshal(userData)
	ioutil.WriteFile(filePath, data, 0644)
}

func setNameHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name      string `json:"name"`
		Breakdown string `json:"breakdown"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	mu.Lock()
	userData["name"] = req.Name
	userData["breakdown"] = req.Breakdown
	mu.Unlock()

	saveUserData()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"name set"}`))
}

func getNameHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mu.Lock()
	name, exists := userData["name"]
	mu.Unlock()

	response := make(map[string]string)
	if exists {
		response["name"] = name
	} else {
		response["name"] = ""
		response["hint"] = "未设置"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}