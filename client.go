package main

import (
	"bytes"
	"encoding/json"
	"errors"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func Post(reqUrl string, auth string, body interface{}) (respBodyObj ResponseBody, err error) {
	postBody, _ := json.Marshal(body)
	requestBody := bytes.NewBuffer(postBody)
	log.WithFields(log.Fields{
		"body": string(postBody),
	}).Debug("The request body")
	req, err := http.NewRequest("POST", reqUrl, requestBody)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(AuthHeaderKey(auth), auth)
	return handleResp(req)
}

func Get(reqUrl string, auth string) (respBodyObj ResponseBody, err error) {
	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(AuthHeaderKey(auth), auth)
	return handleResp(req)
}

func Delete(reqUrl string, auth string) (respBodyObj ResponseBody, err error) {
	req, err := http.NewRequest("DELETE", reqUrl, nil)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(AuthHeaderKey(auth), auth)
	return handleResp(req)
}

func handleResp(req *http.Request) (respBodyObj ResponseBody, err error) {
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	log.WithFields(log.Fields{
		"body": string(respBody),
	}).Debug("The response body")
	err = json.Unmarshal(respBody, &respBodyObj)
	if err != nil {
		log.Fatalln("There was error while parsing the response from server. Exiting...", err)
	}
	if resp.StatusCode != 200 {
		if len(respBodyObj.Message) > 0 {
			log.Error(respBodyObj.Message)
		} else if len(respBodyObj.Messages) > 0 && len(respBodyObj.Messages[0].Message) > 0 {
			log.Error(respBodyObj.Messages[0].Message)
		}
		return respBodyObj, errors.New("received non 200 response code. The response code was " + strconv.Itoa(resp.StatusCode))
	}

	return respBodyObj, nil
}

func AuthHeaderKey(auth string) string {
	if strings.HasPrefix(auth, "Bearer ") {
		return "Authorization"
	}
	return "x-api-key"
}
