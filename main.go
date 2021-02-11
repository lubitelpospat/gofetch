package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/sethgrid/multibar"
)

type BasePathsResponse struct {
	Run_accession string
	Fastq_ftp     string
}

type PassThru struct {
	io.Reader
	progressBar *multibar.ProgressFunc
	Start       time.Time
	total       int // Total # of bytes transferred

}

// Read 'overrides' the underlying io.Reader's Read method.
// This is the one that will be called by io.Copy(). We simply
// use it to keep track of byte counts and then forward the call.
func (pt *PassThru) Read(p []byte) (int, error) {
	n, err := pt.Reader.Read(p)
	pt.total += int(n)
	(*pt.progressBar)(pt.total)
	return n, err
}

func main() {
	srrPtr := flag.String("i", "", "SRA accession or path to file with SRA accessions(requires -L flag)(Required)")
	outDirPtr := flag.String("O", "", "Output directory")

	listFlag := flag.Bool("L", false, "Input is a file with SRA accessions")
	flag.Parse()

	if *srrPtr == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	var directDownloadPaths []string
	if *listFlag == false {
		directDownloadPaths = GetFtpLinks(*srrPtr)
	} else {
		file, err := os.Open(*srrPtr)
		if err != nil {
			fmt.Printf("Couldn't open accession file: \n%s \nfor reading, exiting\n", *srrPtr)
			os.Exit(1)

		}
		var accessions []string
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			accessions = append(accessions, scanner.Text())
		}
		for _, accession := range accessions {
			for _, tmp := range GetFtpLinks(accession) {
				directDownloadPaths = append(directDownloadPaths, tmp)

			}
		}

	}
	//fmt.Println(directDownloadPaths)
	var wg sync.WaitGroup
	numBars := len(directDownloadPaths)
	progressBars, _ := multibar.New()
	wg.Add(numBars)
	for _, fileName := range directDownloadPaths {

		go DownloadFtpFile(fileName, *outDirPtr, &wg, progressBars)

	}
	go progressBars.Listen()
	wg.Wait()
}

func GetFtpLinks(srr string) []string {
	var myClient = &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://www.ebi.ac.uk/ena/portal/api/filereport?result=read_run&fields=fastq_ftp&format=JSON&accession=%s", srr)
	//fmt.Println(url)
	res, err := myClient.Get(url)
	if err != nil {
		fmt.Printf("Accession %s not found, exiting.", srr)
		os.Exit(1)
	}
	defer res.Body.Close()

	var response []BasePathsResponse
	if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
		fmt.Printf("Accession %s not available, exiting.\n", srr)
		os.Exit(1)
	}
	paths := strings.Split(response[0].Fastq_ftp, ";")
	return paths

}

func DownloadFtpFile(remoteFile string, outPath string, wg *sync.WaitGroup, progressBars *multibar.BarContainer) {
	defer wg.Done()

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
	name := fmt.Sprintf("%s:", filepath.Base(localFileName))

	log.Printf("File size: %v bytes", limit)
	r, err := conn.Retr(fileName)
	if err != nil {
		panic(err)
	}
	defer r.Close()
	barProgress := progressBars.MakeBar(int(limit), name)
	response := &PassThru{Reader: r, progressBar: &barProgress, Start: startTime}
	_, err = io.Copy(localFile, response)
	if err != nil {
		panic(err)
	}

	timeElapsed := time.Since(startTime)
	log.Printf("Elapsed\t%v\n", timeElapsed)
}
