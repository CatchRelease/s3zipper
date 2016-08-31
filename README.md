# s3zipper
Microservice that Servers Streaming Zip file from S3 Securely. Uses Postgres to look up the files_hash based upon the unique key.

## Read the blog here
[Original Blog Post](http://engineroom.teamwork.com/how-to-securely-provide-a-zip-download-of-a-s3-file-bundle/)

## Key Format
```
"{UNIQUE_ZIP_ID}" = FILE_HASH.JSON
```

## File Hash Format
```
[{
  FileName: 'Name the file will have in the zip',
  Folder: 'Folder the file will live in inside the zip',
  S3Path: 'Location of file on S3 inside of bucket'
}]
```

## ENVIRONMENT VARIABLES
* AWS_ACCESS_KEY_ID - AWS Access Key
* AWS_SECRET_ACCESS_KEY - AWS Secret Key
* AWS_BUCKET - AWS S3 Bucket
* AWS_REGION - AWS S3 Region
* REDIS - Redis Server and Port (non-heroku)
* DATABASE_URL - Database URL (postgres://username:password@host:port/database)
* PORT - Application server port, defaults to 8000

## TESTING LOCALLY
```
AWS_ACCESS_KEY_ID={FILL_ME_IN} AWS_SECRET_ACCESS_KEY={FILL_ME_IN} AWS_REGION=us-east-1 AWS_BUCKET=catchandrelease-assets-development DATABASE_URL=postgres://username:password@localhost:5432/catchandrelease_development?sslmode=disable go run main.go
```
