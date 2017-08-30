package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"go.uber.org/zap"
)

var Version = "0.0.0"
var Revision = "xxx"

var logger *zap.SugaredLogger
var DryRun bool
var SuppressStderrLogs bool

type AWS_CP_FLAG int

const (
	AWS_CP_SKIP AWS_CP_FLAG = iota
	AWS_CP_COPY
)

func listFiles(searchPath string, fileList chan string, wg *sync.WaitGroup) {
	defer wg.Done()

	pathList, _ := ioutil.ReadDir(searchPath)

	for _, pathObj := range pathList {
		if pathObj.IsDir() {
			childPath := filepath.Join(searchPath, pathObj.Name())

			wg.Add(1)
			go listFiles(childPath, fileList, wg)
		} else {
			fileList <- filepath.Join(searchPath, pathObj.Name())
		}
	}
}

func copyWorker(wg *sync.WaitGroup, rootDir string, fileList chan string, s3Wrapper *S3Wrapper, statusChan chan AWS_CP_FLAG) error {
	defer wg.Done()

	for {
		fromPath, ok := <-fileList
		if !ok {
			return nil
		}

		destinationPart := strings.TrimPrefix(fromPath, rootDir+"/")
		toPath := s3Wrapper.keyPrefix + destinationPart

		var opType string
		if s3Wrapper.HasKey(toPath) {
			opType = "skip"
			statusChan <- AWS_CP_SKIP
		} else {
			opType = "copy"
			statusChan <- AWS_CP_COPY
			if !DryRun {
				_, err := s3Wrapper.PutObject(fromPath, toPath)
				if err != nil {
					logger.Errorw(err.Error())
					return err
				}
			}
		}

		logger.Infow(opType,
			"fromPath", fromPath,
			"toPath", toPath,
		)
	}

	return nil
}

func stderrLog(format string, a ...interface{}) {
	if !SuppressStderrLogs {
		fmt.Fprintf(os.Stderr, format, a...)
	}
}

func main() {
	var fromPath, toBucket, toKeyPrefix, logOutPut string
	var cpConcurrency int

	app := cli.NewApp()
	app.Version = fmt.Sprintf("%s (%s)", Version, Revision)
	app.Name = "S3 Syncer"
	app.Usage = "Sync local data to S3 Bucket"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "from, f",
			Usage:       "Source path to be copied",
			Destination: &fromPath,
		},
		cli.StringFlag{
			Name:        "bucket, b",
			Usage:       "Destination S3 bucket",
			Destination: &toBucket,
		},
		cli.StringFlag{
			Name:        "prefix, k",
			Usage:       "Destination S3 key prefix",
			Value:       "",
			Destination: &toKeyPrefix,
		},
		cli.IntFlag{
			Name:        "con, c",
			Usage:       "Object put concurrency",
			Value:       4,
			Destination: &cpConcurrency,
		},
		cli.BoolTFlag{
			Name:        "do",
			Usage:       "Exec(Default is dry-run)",
			Destination: &DryRun,
		},
		cli.BoolFlag{
			Name:        "suppress, s",
			Usage:       "Suppress STDERR status logs",
			Destination: &SuppressStderrLogs,
		},
		cli.StringFlag{
			Name:        "logs",
			Usage:       "Logs",
			Value:       "stdout",
			Destination: &logOutPut,
		},
	}

	app.Action = func(c *cli.Context) error {
		zapConfig := zap.NewProductionConfig()
		zapConfig.OutputPaths = []string{logOutPut}

		zapLogger, err := zapConfig.Build()

		if err != nil {
			return cli.NewExitError(err, 1)
		}

		defer zapLogger.Sync()

		logger = zapLogger.Sugar()

		var errMsg error
		if fromPath == "" {
			errMsg = errors.WithStack(errors.New("--from is required"))
		}

		if toBucket == "" {
			errMsg = errors.WithStack(errors.New("--bucket is required"))
		}

		if errMsg != nil {
			return cli.NewExitError(errMsg, 1)
		}

		rootPath, _ := filepath.Abs(fromPath)

		logger.Infow("starting",
			"fromPath", rootPath,
			"toBucket", toBucket,
			"toKeyPrefix", toKeyPrefix,
			"concurrency", cpConcurrency,
			"dryrun", DryRun,
		)

		stderrLog("Starting: fromPath=`%s`, toKeyPrefix=`%s`, concurrency=`%d`, dryrun=`%d`\n", fromPath, toKeyPrefix, cpConcurrency, DryRun)

		s3Session := session.Must(session.NewSession())

		s3Svc := s3.New(s3Session)

		s3Wrapper := NewS3Wrapper(s3Svc, toBucket, toKeyPrefix)

		err = s3Wrapper.FetchAllKeys()

		if err != nil {
			return cli.NewExitError(err, 1)
		}

		logger.Infow("cached s3 objects",
			"toBucket", toBucket,
			"toKyePrefix", toKeyPrefix,
			"cacheSize", s3Wrapper.CacheSize(),
		)

		stderrLog("S3 object cached: bucket=`%s`, prefix=`%s`, size=`%d`\n", toBucket, toKeyPrefix, s3Wrapper.CacheSize())

		fileList := make(chan string, 1000)

		var workerWg sync.WaitGroup
		var discoverWg sync.WaitGroup

		statusChan := make(chan AWS_CP_FLAG, 100)

		for i := 0; i < cpConcurrency; i++ {
			workerWg.Add(1)
			go copyWorker(&workerWg, rootPath, fileList, s3Wrapper, statusChan)
		}

		go func() {
			copyCnt := 0
			skipCnt := 0

			for {
				s, ok := <-statusChan

				if !ok {
					return
				}

				switch s {
				case AWS_CP_COPY:
					copyCnt++
				case AWS_CP_SKIP:
					skipCnt++
				}

				msg := "Skip: %d, Copy: %d, All: %d\r"
				stderrLog(msg, skipCnt, copyCnt, skipCnt+copyCnt)
			}
		}()

		discoverWg.Add(1)
		listFiles(rootPath, fileList, &discoverWg)

		discoverWg.Wait()
		close(fileList)
		workerWg.Wait()
		close(statusChan)

		return nil
	}

	app.Run(os.Args)
}
