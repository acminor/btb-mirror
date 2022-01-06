/*
 * Application command. See btb.go for a description.
 *
 * Author: A.C. Minor
 * SPDX identifier: BSD-3-Clause
 */

package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Args struct {
	BinPath     string
	Prefix      string
	Container   string
	InContainer bool
}

func currentExePath() string {
	currentExePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	currentExePath, err = filepath.EvalSymlinks(currentExePath)
	if err != nil {
		log.Fatal(err)
	}

	return currentExePath
}

func dirExists(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	} else if err != nil {
		log.Fatal(err)
	}

	return true
}

func inPlaceReverse(arr []string) {
	size := len(arr)
	midPoint := size / 2

	for i := 0; i < midPoint; i++ {
		arr[i], arr[size-1-i] = arr[size-1-i], arr[i]
	}
}

func canExecute(userInfo *user.User, info os.FileInfo) bool {
	// from man chmod(1p)
	const S_IXUSR = 0100
	const S_IXGRP = 0010
	const S_IXOTH = 0001

	mode := info.Mode()

	if (S_IXOTH & mode) != 0 {
		return true
	}

	unixInfo, ok := info.Sys().(syscall.Stat_t)
	if !ok {
		return false
	}

	userGid, err := strconv.ParseUint(userInfo.Gid, 10, 32)
	if err != nil {
		log.Fatal(err)
	}

	if (S_IXGRP&mode) != 0 && unixInfo.Gid == uint32(userGid) {
		return true
	}

	userId, err := strconv.ParseUint(userInfo.Uid, 10, 32)
	if err != nil {
		log.Fatal(err)
	}

	if (S_IXUSR&mode) != 0 && unixInfo.Uid == uint32(userId) {
		return true
	}

	return false
}

const BinFormat = `#!/usr/bin/env bash

toolbox run -c %s %s $@
`

var rootCmd = &cobra.Command{
	Use:   "temp",
	Short: "Temp",
	Long:  `Temp`,
	Run:   rootCommandFunction,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(-1)
	}
}

var args Args

func init() {
	rootCmd.Flags().StringVarP(&args.BinPath, "binpath", "", "", "TODO")
	rootCmd.Flags().StringVarP(&args.Prefix, "prefix", "", "", "TODO")
	rootCmd.Flags().StringVarP(&args.Container, "container", "", "", "TODO")
	rootCmd.Flags().BoolVarP(&args.InContainer, "in-container", "", false, "TODO")

	rootCmd.MarkFlagRequired("binpath")
	rootCmd.MarkFlagRequired("prefix")
	rootCmd.MarkFlagRequired("container")
}

func rootCommandFunction(_ *cobra.Command, _ []string) {
	currentExePath := currentExePath()

	if !args.InContainer {
		toolboxArgs := []string{"run", "-c", args.Container, "/usr/bin/zsh"} //, "-c"}
		inContainer := "true"
		programArgs := []string{
			currentExePath,
			"--binpath", args.BinPath,
			"--prefix", args.Prefix,
			"--container", args.Container,
			"--in-container", inContainer,
		}
		execProgram := strings.Join(append(programArgs, "\n"), " ")

		ctx, cancel := context.WithTimeout(context.Background(), 30000*time.Millisecond)

		cmd := exec.CommandContext(ctx, "toolbox", toolboxArgs...)

		stdin, _ := cmd.StdinPipe()
		stdout, _ := cmd.StdoutPipe()

		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()

		err := cmd.Start()
		if err != nil {
			log.Fatal(err)
		}

		stdin.Write([]byte(execProgram))

		go func() {
			reader := bufio.NewReader(os.Stdin)
			for {
				data, _ := reader.ReadBytes('\n')
				stdin.Write(data)
			}
		}()

		go func() {
			for {
				// cannot use buffered reading b/c prompt for rmdir is not newline outputted
				data := make([]byte, 4096)
				i, err := stdout.Read(data)
				if err != nil {
					log.Fatal(err)
				}

				if i == 0 {
					continue
				}

				if strings.Contains(string(data), "<<<Done>>>") {
					stdin.Write([]byte("exit\n"))
					return
				} else if strings.Contains(string(data), execProgram) {
				} else {
					fmt.Print(string(data))
				}
			}
		}()

		if err := cmd.Wait(); err != nil {
			log.Fatal(err)
		}

		cancel()
		os.Exit(0)
	}

	pathEnv := os.Getenv("PATH")
	paths := []string{}
	for _, path := range strings.Split(pathEnv, ":") {
		if dirExists(path) {
			var isBtbPath bool
			if err := filepath.WalkDir(path, func(_ string, dirEntry os.DirEntry, _ error) error {
				if dirEntry.Name() != filepath.Base(path) && dirEntry.IsDir() { // do not recurse in internal dirs
					return filepath.SkipDir
				}

				if dirEntry.Name() == ".btbMarker" {
					isBtbPath = true
				}
				return nil
			}); err != nil {
				log.Fatal(err)
			}

			if !isBtbPath {
				paths = append(paths, path)
			}
		}
	}

	reader := bufio.NewReader(os.Stdin)

	binPath := filepath.Join(args.BinPath, args.Prefix)
	if dirExists(binPath) {
		fmt.Printf("rmdir: %s (y/n)? ", binPath)

		incorrectEntryCount := 0
	UserInputLoop:
		for {
			response, err := reader.ReadString('\n')
			if err != nil {
				log.Fatal(err)
			}

			switch strings.TrimSpace(strings.ToLower(response)) {
			case "y", "yes":
				if err := os.RemoveAll(binPath); err != nil {
					log.Fatal(err)
				}
				break UserInputLoop
			case "n", "no":
				log.Fatal("Cannot continue with non-empty directory")
			default:
				if incorrectEntryCount == 3 {
					log.Fatal("Too many incorrect tries. Stopping")
				}
				fmt.Print("Please enter (y/n): ")
				incorrectEntryCount++
			}
		}
	}

	parentStat, err := os.Stat(args.BinPath)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.Mkdir(binPath, parentStat.Mode()); err != nil {
		log.Fatal(err)
	}

	btbMarkerFile, err :=
		os.OpenFile(filepath.Join(binPath, ".btbMarker"), os.O_CREATE, parentStat.Mode())
	if err != nil {
		log.Fatal(err)
	}
	if err := btbMarkerFile.Close(); err != nil {
		log.Fatal(err)
	}
	btbMarkerFile.Close()

	var allExe []string
	inPlaceReverse(paths)
	for _, path := range paths {
		if err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if d.Name() != filepath.Base(path) && d.IsDir() { // do not recurse in internal dirs
				return filepath.SkipDir
			}

			if err != nil {
				return err
			}

			currentUser, err := user.Current()
			if err != nil {
				return err
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			if !d.IsDir() && canExecute(currentUser, info) {
				allExe = append(allExe, p)
			}

			return nil
		}); err != nil {
			log.Fatal(err)
		}
	}

	exeMap := make(map[string]string)
	for _, exePath := range allExe {
		exe := filepath.Base(exePath)
		exeMap[exe] = exePath
	}

	for exe, exePath := range exeMap {
		fileName := fmt.Sprintf("%s-%s", args.Prefix, exe)
		filePath := filepath.Join(binPath, fileName)

		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, parentStat.Mode())
		if err != nil {
			log.Fatal(err)
		}

		fileContents := fmt.Sprintf(BinFormat, args.Container, exePath)
		if _, err := file.WriteString(fileContents); err != nil {
			log.Fatal(err)
		}

		if err := file.Close(); err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("<<<Done>>>")
}
