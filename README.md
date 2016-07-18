# s3zipper
Microservice that Servers Streaming Zip file from S3 Securely. Uses Redis to look up the files_hash based upon the unique id.

## Read the blog here
[Original Blog Post](http://engineroom.teamwork.com/how-to-securely-provide-a-zip-download-of-a-s3-file-bundle/)

## Key Format
```
"zip:#{UNIQUE_ZIP_ID}" = FILE_HASH.JSON
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
* REDISTOGO_URL - Heroku Redis Config
* PORT - Application server port, defaults to 8000
