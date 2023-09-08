package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

func Post(reqUrl string, auth string, body interface{}, contentType string, requestBodyWithFile *bytes.Buffer) (respBodyObj ResponseBody, err error) {
	postBody, _ := json.Marshal(body)
	requestBody := bytes.NewBuffer(postBody)
	var req *http.Request
	fmt.Println("requestBody")
	printJson(requestBody)
	log.WithFields(log.Fields{
		"body": string(postBody),
	}).Debug("The request body")

	if requestBodyWithFile != nil {
		requestBody = requestBodyWithFile
	}

	req, err = http.NewRequest("POST", reqUrl, requestBody)

	if err != nil {
		return
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(AuthHeaderKey(auth), auth)

	if err != nil {
		log.Fatalln(err)

	}

	return handleResp(req)
}

func Put(reqUrl string, auth string, body interface{}, contentType string, requestBodyWithFile *bytes.Buffer) (respBodyObj ResponseBody, err error) {
	postBody, _ := json.Marshal(body)
	requestBody := bytes.NewBuffer(postBody)
	var req *http.Request
	log.WithFields(log.Fields{
		"body": string(postBody),
	}).Debug("The request body")
	if requestBodyWithFile != nil {
		requestBody = requestBodyWithFile
	}

	req, err = http.NewRequest("PUT", reqUrl, requestBody)

	if err != nil {
		return
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(AuthHeaderKey(auth), auth)
	return handleResp(req)
}

func Get(reqUrl string, auth string) (respBodyObj ResponseBody, err error) {
	req, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req.Header.Set(AuthHeaderKey(auth), cliCdRequestData.AuthToken)

	return handleResp(req)
}

func Delete(reqUrl string, auth string) (respBodyObj ResponseBody, err error) {
	req, err := http.NewRequest("DELETE", reqUrl, nil)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", CONTENT_TYPE_JSON)
	req.Header.Set(AuthHeaderKey(auth), auth)
	return handleResp(req)
}

func handleResp(req *http.Request) (respBodyObj ResponseBody, err error) {
	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		log.Fatalln(err)
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(respBody, &respBodyObj)
	if err != nil {
		log.Fatalln("There was error while parsing the response from server. Exiting...", err)
	}
	if resp.StatusCode != 200 {
		if resp.StatusCode >= 400 || resp.StatusCode < 500 {
			println(respBodyObj.Message)
		} else {
			if len(respBodyObj.Message) > 0 {
				log.Error(respBodyObj.Message)
			} else if len(respBodyObj.Messages) > 0 && len(respBodyObj.Messages[0].Message) > 0 {
				log.Error(respBodyObj.Messages[0].Message)
			}
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
