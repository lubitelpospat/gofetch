package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

type BasePathsResponse struct {
	Run_accession string
	Fastq_ftp     string
}

func main() {
	srr := os.Args[1]
	directDownloadPaths := GetFtpLinks(srr)
	//fmt.Println(directDownloadPaths)
	for _, fileName := range directDownloadPaths {
		fmt.Println(fileName)
		DownloadFtpFile(fileName)

	}
}

func GetFtpLinks(srr string) []string {
	var myClient = &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://www.ebi.ac.uk/ena/portal/api/filereport?result=read_run&fields=fastq_ftp&format=JSON&accession=%s", srr)
	//fmt.Println(url)
	res, err := myClient.Get(url)
	if err != nil {
		panic(err.Error())
	}
	defer res.Body.Close()

	var response []BasePathsResponse
	if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
		panic(err.Error())
	}
	paths := strings.Split(response[0].Fastq_ftp, ";")
	return paths

}

func DownloadFtpFile(remoteFile string) {

	startTime := time.Now()

	localFileName := filepath.Base(remoteFile)
	log.Printf("Start download:%v\r\n", remoteFile)
	localFile, err := os.Create(localFileName)
	defer localFile.Close()

	serverName := strings.Split(remoteFile, "/")[0]
	fileName := strings.Join(strings.Split(remoteFile, "/")[1:], "/")
	log.Printf("File name: %s\n", fileName)
	log.Printf("Server name: %s\n", serverName)
	conn, err := ftp.Dial(fmt.Sprintf("%s:21", serverName), ftp.DialWithTimeout(5*time.Second))
	if err != nil {
		log.Fatal(err)
	}
	if err = conn.Login("anonymous", "anonymous"); err != nil {
		panic(err)
	}
	r, err := conn.Retr(fileName)
	if err != nil {
		panic(err)
	}
	defer r.Close()
	_, err = io.Copy(localFile, r)
	if err != nil {
		panic(err)
	}

	timeElapsed := time.Since(startTime)
	log.Println("Elapsed\t%v", timeElapsed)
}
