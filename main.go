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
	"github.com/vbauerster/mpb"
	"github.com/vbauerster/mpb/decor"
)

type BasePathsResponse struct {
	Run_accession string
	Fastq_ftp     string
}

type PassThru struct {
	io.Reader
	bar   *mpb.Bar
	start time.Time
	//progressBar *multibar.ProgressFunc
	total int // Total # of bytes transferred

}

// Read 'overrides' the underlying io.Reader's Read method.
// This is the one that will be called by io.Copy(). We simply
// use it to keep track of byte counts and then forward the call.
func (pt *PassThru) Read(p []byte) (int, error) {
	n, err := pt.Reader.Read(p)
	pt.total += int(n)
	pt.bar.IncrBy(n)
	pt.bar.DecoratorEwmaUpdate(time.Since(pt.start))
	//(*pt.progressBar)(pt.total)
	return n, err
}

func main() {

	srrPtr := flag.String("i", "", "SRA accession or path to file with SRA accessions(requires -L flag)(Required)")
	outDirPtr := flag.String("O", "", "Output directory, default \"\"")
	numWorkersPtr := flag.Int("t", 5, "Number of workers to use for downloading, default 5.")
	//timeoutPtr := flag.Int("T", 10, "Timeout for FTP connections in seconds, default 10")

	listFlag := flag.Bool("L", false, "Input is a file with SRA accessions")
	flag.Parse()
	numWorkers := *numWorkersPtr
	if *srrPtr == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	var directDownloadPaths []string
	if *listFlag == false {
		log.Println("Getting information about file locations, please wait...")
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
		log.Println("Getting information about file locations, please wait...")
		for _, accession := range accessions {
			for _, tmp := range GetFtpLinks(accession) {
				directDownloadPaths = append(directDownloadPaths, tmp)

			}
		}

	}

	if len(directDownloadPaths) < numWorkers {
		numWorkers = len(directDownloadPaths)
	}

	//fmt.Println(directDownloadPaths)
	//wg := new(sync.WaitGroup)
	doneWg := new(sync.WaitGroup)
	p := mpb.New(mpb.WithWaitGroup(doneWg))
	//var bars []*mpb.Bar
	log.Printf("num workers: %d\n", numWorkers)
	//wg.Add(numWorkers)
	log.Printf("%d files are to be downloaded.\n", len(directDownloadPaths))
	resultsChannel := make(chan int)
	taskChannel := make(chan string)
	for i := 0; i < numWorkers; i++ {
		go DownloadWorker(*outDirPtr, p, &taskChannel, &resultsChannel)
	}

	go func() {
		for _, fileName := range directDownloadPaths {
			taskChannel <- fileName

		}
	}()

	for i := 0; i < len(directDownloadPaths); i++ {
		<-resultsChannel
	}
	p.Wait()
	close(taskChannel)
	close(resultsChannel)

}

func DownloadWorker(outPath string, p *mpb.Progress, taskChannel *chan string, resultsChannel *chan int) {
	for task := range *taskChannel {
		DownloadFtpFile(task, outPath, p)
		*resultsChannel <- -1

	}
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

func DownloadFtpFile(remoteFile string, outPath string, p *mpb.Progress) {

	localFileName := filepath.Base(remoteFile)

	localFile, err := os.Create(filepath.Join(outPath, localFileName))
	if err != nil {
		fmt.Println("Error creating local file. Check that output directory exists and all write permissions are satisfied. \nExiting.")
	}
	defer localFile.Close()

	serverName := strings.Split(remoteFile, "/")[0]
	fileName := strings.Join(strings.Split(remoteFile, "/")[1:], "/")

	conn, err := ftp.Dial(fmt.Sprintf("%s:21", serverName), ftp.DialWithTimeout(10*time.Second))
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
	name := fmt.Sprintf("%s:", localFileName)

	//log.Printf("File size: %v bytes", limit)
	r, err := conn.Retr(fileName)
	if err != nil {
		panic(err)
	}
	defer r.Close()
	//barProgress := progressBars.MakeBar(int(limit), name)
	//response := &PassThru{Reader: r, progressBar: &barProgress}
	newBar := p.AddBar(limit,
		mpb.PrependDecorators(
			decor.Name(name, decor.WC{W: len(name) + 1, C: decor.DidentRight}),
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
		),
		mpb.AppendDecorators(decor.Percentage(decor.WC{W: 5})),
	)
	//*bars = append(*bars, newBar)
	response := &PassThru{Reader: r, bar: newBar, start: time.Now()}
	_, err = io.Copy(localFile, response)
	if err != nil {
		panic(err)
	}

	//timeElapsed := time.Since(startTime)
	//log.Printf("Elapsed\t%v\n", timeElapsed)
}
