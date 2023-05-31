package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

func Update(*cli.Context) (err error) {
	newVersion := GetNewRelease()
	if len(newVersion) == 0 {
		fmt.Println("Already on latest version. Skipping update")
		return
	}
	blue := color.New(color.FgHiBlue).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Printf("New version %s is available.\n", green(newVersion))
	confirm := ConfirmInput("Do you want to update?")
	if !confirm {
		return nil
	}
	extension := "tar.gz"
	const GOOS = runtime.GOOS
	const GOARCH = runtime.GOARCH
	if GOOS == "windows" {
		extension = "zip"
	}
	url := fmt.Sprintf("https://github.com/harness/migrator/releases/download/%s/harness-upgrade-%s-%s-%s.%s", newVersion, newVersion, GOOS, GOARCH, extension)

	if GOOS == "windows" {
		fmt.Printf("%s\n", yellow("Auto update support is not available for windows"))
		fmt.Printf("Download the following release - %s\n", blue(url))
		return nil
	}

	ex, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := filepath.Abs(path.Dir(ex))
	if err != nil {
		return err
	}

	// Download the file
	fmt.Printf("Downloading the following - %s\n", blue(url))
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			return
		}
	}(resp.Body)

	fmt.Printf("Unpacking the contents and extracting to - %s\n", blue(dir))
	err = readTar(resp.Body, dir)
	if err == nil {
		fmt.Printf("Successfully upgraded to - %s\n", green(newVersion))
	}
	return err
}

func readTar(body io.ReadCloser, dest string) error {
	gzRead, err := gzip.NewReader(body)
	if err != nil {
		return err
	}
	reader := tar.NewReader(gzRead)
	for true {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg && header.Name == "harness-upgrade" {
			execFile := path.Join(dest, header.Name)
			green := color.New(color.FgGreen).SprintFunc()
			fmt.Printf("Extracting harness-upgrade to - %s\n", green(execFile))
			outFile, err := os.Create(execFile)
			if err != nil {
				return err
			}
			if _, err = io.Copy(outFile, reader); err != nil {
				return err
			}
			err = outFile.Close()
			if err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}
