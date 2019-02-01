package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/s3"
	_ "github.com/lib/pq"
	"github.com/rollbar/rollbar-go"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Configuration struct {
	AccessKey    string
	SecretKey    string
	Bucket       string
	Region       string
	Database     string
	Port         int
	RollbarToken string
	Environment  string
	GitCommit    string
	GithubRepo   string
}

var config = Configuration{}
var aws_bucket *s3.Bucket

type FileHash struct {
	FileName     string
	Folder       string
	S3Path       string
	FileId       int64 `json:",string"`
	ProjectId    int64 `json:",string"`
	ProjectName  string
	Modified     string
	ModifiedTime time.Time
}

var (
	db *sql.DB
)

func main() {
	if 1 == 0 {
		test()
		return
	}

	port, err := strconv.Atoi(os.Getenv("PORT"))

	if err != nil {
		port = 8000
	}

	config = Configuration{
		AccessKey:    os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretKey:    os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Bucket:       os.Getenv("AWS_BUCKET"),
		Region:       os.Getenv("AWS_REGION"),
		Database:     os.Getenv("DATABASE_URL"),
		Port:         port,
		RollbarToken: os.Getenv("ROLLBAR_KEY"),
		Environment:  getenvOrDefault("APP_ENV", "development"),
		GitCommit:    os.Getenv("SOURCE_VERSION"),
		GithubRepo:   "github.com/CatchRelease/s3zipper",
	}

	fmt.Println("ENVIRONMENT", config.Environment)
	fmt.Println("AWS ACCESS KEY ID", config.AccessKey)
	fmt.Println("AWS SECRET ACCESS KEY", config.SecretKey)
	fmt.Println("AWS REGION", config.Region)
	fmt.Println("AWS BUCKET", config.Bucket)
	fmt.Println("DATABASE_URL", config.Database)

	initRollbar()

	var finishSetupAndListen = func() {
		initAwsBucket()
		initDB()

		fmt.Println("Running on port", config.Port)
		http.HandleFunc("/", handler)
		log.Fatal(http.ListenAndServe(":"+strconv.Itoa(config.Port), nil))
	}

	if config.RollbarToken != "" {
		rollbar.WrapAndWait(finishSetupAndListen)
	} else {
		finishSetupAndListen()
	}
}

func test() {
	var err error
	var files []*FileHash
	jsonData := "[{\"S3Path\":\"1\\/p23216.tf_A89A5199-F04D-A2DE-5824E635AC398956.Avis_Rent_A_Car_Print_Reservation.pdf\",\"FileVersionId\":\"4164\",\"FileName\":\"Avis Rent A Car_ Print Reservation.pdf\",\"ProjectName\":\"Superman\",\"ProjectId\":\"23216\",\"Folder\":\"\",\"FileId\":\"4169\"},{\"modified\":\"2015-07-18T02:05:04Z\",\"S3Path\":\"1\\/p23216.tf_351310E0-DF49-701F-60601109C2792187.a1.jpg\",\"FileVersionId\":\"4165\",\"FileName\":\"a1.jpg\",\"ProjectName\":\"Superman\",\"ProjectId\":\"23216\",\"Folder\":\"Level 1\\/Level 2 x\\/Level 3\",\"FileId\":\"4170\"}]"

	resultByte := []byte(jsonData)

	err = json.Unmarshal(resultByte, &files)
	if err != nil {
		err = errors.New("Error decoding json: " + jsonData)
	}

	parseFileDates(files)
}

func parseFileDates(files []*FileHash) {
	layout := "2006-01-02T15:04:05Z"
	for _, file := range files {
		t, err := time.Parse(layout, file.Modified)
		if err != nil {
			fmt.Println(err)
			continue
		}
		file.ModifiedTime = t
	}
}

func getenvOrDefault(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = fallback
	}
	return value
}

func initRollbar() {
	rollbar.SetToken(config.RollbarToken)
	rollbar.SetEnvironment(config.Environment)
	rollbar.SetCodeVersion(config.GitCommit)
	rollbar.SetServerRoot(config.GithubRepo)
}

func initAwsBucket() {
	expiration := time.Now().Add(time.Hour * 1)
	auth, err := aws.GetAuth(config.AccessKey, config.SecretKey, "", expiration) //"" = token which isn't needed
	if err != nil {
		panic(err)
	}

	aws_bucket = s3.New(auth, aws.GetRegion(config.Region)).Bucket(config.Bucket)
}

func initDB() {
	var err error

	db, err = sql.Open("postgres", config.Database)
	if err != nil {
		panic(err)
	}

	err = db.Ping()
	if err != nil {
		panic(err)
	}
}

// Remove all other unrecognised characters apart from
var makeSafeFileName = regexp.MustCompile(`[#<>:"/\|?*\\]`)

func getFilesFromDB(ref string) (files []*FileHash, err error) {
	var result string

	db_err := db.QueryRow("SELECT files_hash FROM batch_downloads WHERE key = $1", ref).Scan(&result)

	switch {
	case db_err == sql.ErrNoRows:
		err = errors.New("Could not find that batch download.")
		return
	case db_err != nil:
		err = db_err
		return
	}

	if result == "" {
		err = errors.New("Could not find that batch download.")
		return
	}

	// Decode JSON
	err = json.Unmarshal([]byte(result), &files)
	if err != nil {
		err = errors.New("Error decoding json: " + string(result))
	}

	return
}

func handler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Get "ref" URL params
	refs, ok := r.URL.Query()["ref"]
	if !ok || len(refs) < 1 {
		http.Error(w, "S3 File Zipper. Pass ?ref= to use.", 500)
		return
	}
	ref := refs[0]

	// Get "downloadas" URL params
	downloadas, ok := r.URL.Query()["downloadas"]
	if !ok && len(downloadas) > 0 {
		downloadas[0] = makeSafeFileName.ReplaceAllString(downloadas[0], "")
		if downloadas[0] == "" {
			downloadas[0] = "download.zip"
		}
	} else {
		downloadas = append(downloadas, "download.zip")
	}

	files, err := getFilesFromDB(ref)
	if err != nil {
		http.Error(w, err.Error(), 403)
		log.Printf("%s\t%s\t%s", r.Method, r.RequestURI, err.Error())
		return
	}

	// Start processing the response
	w.Header().Add("Content-Disposition", "attachment; filename=\""+downloadas[0]+"\"")
	w.Header().Add("Content-Type", "application/zip")

	// Loop over files, add them to the
	zipWriter := zip.NewWriter(w)
	for _, file := range files {

		if file.S3Path == "" {
			log.Printf("Missing path for file: %v", file)
			continue
		}

		// Build safe file file name
		safeFileName := makeSafeFileName.ReplaceAllString(file.FileName, "")
		if safeFileName == "" { // Unlikely but just in case
			safeFileName = "file"
		}

		// Read file from S3, log any errors
		rdr, err := aws_bucket.GetReader(file.S3Path)
		if err != nil {
			switch t := err.(type) {
			case *s3.Error:
				if t.StatusCode == 404 {
					log.Printf("File not found. %s", file.S3Path)
				}
			default:
				log.Printf("Error downloading \"%s\" - %s", file.S3Path, err.Error())
			}
			continue
		}

		// Build a good path for the file within the zip
		zipPath := ""
		// Prefix project Id and name, if any (remove if you don't need)
		if file.ProjectId > 0 {
			zipPath += strconv.FormatInt(file.ProjectId, 10) + "."
			// Build Safe Project Name
			file.ProjectName = makeSafeFileName.ReplaceAllString(file.ProjectName, "")
			if file.ProjectName == "" { // Unlikely but just in case
				file.ProjectName = "Project"
			}
			zipPath += file.ProjectName + "/"
		}
		// Prefix folder name, if any
		if file.Folder != "" {
			zipPath += file.Folder
			if !strings.HasSuffix(zipPath, "/") {
				zipPath += "/"
			}
		}
		zipPath += safeFileName

		// We have to set a special flag so zip files recognize utf file names
		// See http://stackoverflow.com/questions/30026083/creating-a-zip-archive-with-unicode-filenames-using-gos-archive-zip
		h := &zip.FileHeader{
			Name:   zipPath,
			Method: zip.Deflate,
			Flags:  0x800,
		}

		if file.Modified != "" {
			h.SetModTime(file.ModifiedTime)
		}

		f, _ := zipWriter.CreateHeader(h)

		io.Copy(f, rdr)
		rdr.Close()
	}

	zipWriter.Close()

	log.Printf("%s\t%s\t%s", r.Method, r.RequestURI, time.Since(start))
}
