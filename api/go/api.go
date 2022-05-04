package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/gorilla/mux"
)

// API是一个单独的服务，可以称为API服务，以Restful接口服务的方式提供
// 任何语言都可以通过常规的Restful服务接口调用的方式来调用这些接口
// 调用这些接口无需提供token授权，因此完全开放
type SendBody struct {
	SendTo   string  `json:"sendto"`
	SendFrom string  `json:"sendfrom"`
	Amount   float64 `json:"amount"`
}

const (
	URL  = "http://localhost:5000/_jsonrpc"
	PORT = ":8000"
)

func getBlockchain(w http.ResponseWriter, r *http.Request) {
	var jsonStr = []byte(`{
		"id": 1,
		"method": "API.GetBlockchain", 
		"params": []
	}`)
	req, err := http.NewRequest("POST", URL, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Info("response Status:", resp.Status)
	log.Info("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Info("response Body:", string(body))

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	if err != nil {
		log.Errorf("写入 response body 失败: %v", err)
		return
	}
}

func getBalance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	byt := fmt.Sprintf(`{
		"id": 1,
		"method": "API.GetBalance", 
		"params": [{"Address": "%s"}]
	}`, vars["address"])

	var jsonStr = []byte(byt)
	req, err := http.NewRequest("POST", URL, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Info("response Status:", resp.Status)
	log.Info("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Info("response Body:", string(body))

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	if err != nil {
		log.Errorf("写入 response body 失败: %v", err)
		return
	}
}

func send(w http.ResponseWriter, r *http.Request) {
	var respBody SendBody
	err := json.NewDecoder(r.Body).Decode(&respBody)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fmt.Println(respBody)
	byt := fmt.Sprintf(`{"id": 1 , "method": "API.Send", "params": [{"sendFrom":"%s","sendTo": "%s", "amount": %f}]}`, respBody.SendFrom, respBody.SendTo, respBody.Amount)
	var jsonStr = []byte(byt)
	req, err := http.NewRequest("POST", URL, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	log.Info("response Status:", resp.Status)
	log.Info("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	log.Info("response Body:", string(body))

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	if err != nil {
		log.Errorf("写入 response body 失败: %v", err)
		return
	}
}
func main() {
	router := mux.NewRouter()
	router.HandleFunc("/getblockchain", getBlockchain).Methods("GET")
	router.HandleFunc("/getbalance/{address}", getBalance).Methods("GET")
	router.HandleFunc("/send", send).Methods("POST")

	log.Info("正在监听的端口： " + PORT)
	log.Fatalln(http.ListenAndServe(PORT, router))
}
