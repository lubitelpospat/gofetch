package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cheggaaa/pb/v3"
	"github.com/jlaffaye/ftp"
)

type BasePathsResponse struct {
	Run_accession string
	Fastq_ftp     string
}

func main() {
	srrPtr := flag.String("i", "", "SRA accession (Required)")
	outDirPtr := flag.String("O", "", "Output directory")
	flag.Parse()

	if *srrPtr == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	directDownloadPaths := GetFtpLinks(*srrPtr)
	//fmt.Println(directDownloadPaths)
	for _, fileName := range directDownloadPaths {
		log.Printf("Start download:%v\r\n", fileName)
		DownloadFtpFile(fileName, *outDirPtr)

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

func DownloadFtpFile(remoteFile string, outPath string) {

	startTime := time.Now()

	localFileName := filepath.Base(remoteFile)

	localFile, err := os.Create(filepath.Join(outPath, localFileName))
	if err != nil {
		fmt.Println("Error creating local file. Check that output directory exists and all write permissions are satisfied. \nExiting.")
	}
	defer localFile.Close()

	serverName := strings.Split(remoteFile, "/")[0]
	fileName := strings.Join(strings.Split(remoteFile, "/")[1:], "/")

	conn, err := ftp.Dial(fmt.Sprintf("%s:21", serverName), ftp.DialWithTimeout(5*time.Second))
	if err != nil {
		log.Fatal(err)
	}
	if err = conn.Login("anonymous", "anonymous"); err != nil {
		panic(err)
	}
	limit, err := conn.FileSize(fileName)
	if err != nil {
		fmt.Println("Error estimating file size.\nExiting.")
		os.Exit(1)
	}
	log.Printf("File size: %v bytes", limit)
	r, err := conn.Retr(fileName)
	if err != nil {
		panic(err)
	}
	defer r.Close()

	bar := pb.Full.Start64(limit)
	defer bar.Finish()
	barReader := bar.NewProxyReader(r)
	_, err = io.Copy(localFile, barReader)
	if err != nil {
		panic(err)
	}

	timeElapsed := time.Since(startTime)
	log.Printf("Elapsed\t%v\n", timeElapsed)
}
