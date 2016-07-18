package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"net/http"
	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/s3"
	"github.com/soveran/redisurl"
	"github.com/garyburd/redigo/redis"
)

type Configuration struct {
	AccessKey          string
	SecretKey          string
	Bucket             string
	Region             string
	Redis              string
	RedisServerAndPort string
	Port               int
}

var config = Configuration{}
var aws_bucket *s3.Bucket
var redisPool *redis.Pool

type RedisFile struct {
	FileName string
	Folder   string
	S3Path   string
	FileId       int64 `json:",string"`
	ProjectId    int64 `json:",string"`
	ProjectName  string
	Modified     string
	ModifiedTime time.Time
}

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
		AccessKey: os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		Bucket: os.Getenv("AWS_BUCKET"),
		Region: os.Getenv("AWS_REGION"),
		Redis: os.Getenv("REDIS"),
		RedisServerAndPort: os.Getenv("REDISTOGO_URL"),
		Port: port,
	}

	fmt.Println("AWS ACCESS KEY", config.AccessKey)
	fmt.Println("AWS SECRET KEY", config.SecretKey)
	fmt.Println("AWS REGION", config.Region)
	fmt.Println("AWS BUCKET", config.Bucket)
	fmt.Println("REDIS", config.Redis)
	fmt.Println("REDISTOGO_URL", config.RedisServerAndPort)

	initAwsBucket()
	InitRedis()

	fmt.Println("Running on port", config.Port)
	http.HandleFunc("/", handler)
	http.ListenAndServe(":"+strconv.Itoa(config.Port), nil)
}

func test() {
	var err error
	var files []*RedisFile
	jsonData := "[{\"S3Path\":\"1\\/p23216.tf_A89A5199-F04D-A2DE-5824E635AC398956.Avis_Rent_A_Car_Print_Reservation.pdf\",\"FileVersionId\":\"4164\",\"FileName\":\"Avis Rent A Car_ Print Reservation.pdf\",\"ProjectName\":\"Superman\",\"ProjectId\":\"23216\",\"Folder\":\"\",\"FileId\":\"4169\"},{\"modified\":\"2015-07-18T02:05:04Z\",\"S3Path\":\"1\\/p23216.tf_351310E0-DF49-701F-60601109C2792187.a1.jpg\",\"FileVersionId\":\"4165\",\"FileName\":\"a1.jpg\",\"ProjectName\":\"Superman\",\"ProjectId\":\"23216\",\"Folder\":\"Level 1\\/Level 2 x\\/Level 3\",\"FileId\":\"4170\"}]"

	resultByte := []byte(jsonData)

	err = json.Unmarshal(resultByte, &files)
	if err != nil {
		err = errors.New("Error decoding json: " + jsonData)
	}

	parseFileDates(files)
}

func parseFileDates(files []*RedisFile) {
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

func initAwsBucket() {
	expiration := time.Now().Add(time.Hour * 1)
	auth, err := aws.GetAuth(config.AccessKey, config.SecretKey, "", expiration) //"" = token which isn't needed
	if err != nil {
		panic(err)
	}

	aws_bucket = s3.New(auth, aws.GetRegion(config.Region)).Bucket(config.Bucket)
}

func InitRedis() {
	redisPool = &redis.Pool{
		MaxIdle:     10,
		IdleTimeout: 1 * time.Second,
		Dial: func() (redis.Conn, error) {
			if config.Redis != "" {
				fmt.Println("Using normal Redis", config.Redis)
				return redis.Dial("tcp", config.Redis)
			} else {
				fmt.Println("Using RedisToGo Redis", config.RedisServerAndPort)
				return redisurl.ConnectToURL(config.RedisServerAndPort)
			}
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) (err error) {
			_, err = c.Do("PING")
			if err != nil {
				panic("Error connecting to redis")
			}
			return
		},
	}
}

// Remove all other unrecognised characters apart from
var makeSafeFileName = regexp.MustCompile(`[#<>:"/\|?*\\]`)

func getFilesFromRedis(ref string) (files []*RedisFile, err error) {
	redis := redisPool.Get()
	defer redis.Close()

	// Get the value from Redis
	result, err := redis.Do("GET", "zip:"+ref)
	if err != nil || result == nil {
		err = errors.New("Access Denied (sorry your link has timed out)")
		return
	}

	// Convert to bytes
	var resultByte []byte
	var ok bool
	if resultByte, ok = result.([]byte); !ok {
		err = errors.New("Error converting data stream to bytes")
		return
	}

	// Decode JSON
	err = json.Unmarshal(resultByte, &files)
	if err != nil {
		err = errors.New("Error decoding json: " + string(resultByte))
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

	files, err := getFilesFromRedis(ref)
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
