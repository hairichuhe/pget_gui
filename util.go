package uget

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/ricochet2200/go-disk-usage/du"
	"gopkg.in/cheggaaa/pb.v1"
)

// Data struct has file of relational data  文件相关数据
type Data struct {
	filename     string
	filesize     uint
	dirname      string
	fullfilename string
}

// Utils interface indicate function
type Utils interface {
	ProgressBar(context.Context) error
	BindwithFiles(int) error
	IsFree(uint) error
	Progress(string) (int64, error)
	MakeRange(uint, uint, uint) Range
	URLFileName(string, string) string

	// like setter
	SetFileName(string)
	SetFullFileName(string, string)
	SetDirName(string, string, int)
	SetFileSize(uint)

	// like getter
	FileName() string
	FullFileName() string
	FileSize() uint
	DirName() string
}

func isDos() bool {
	return runtime.GOOS == "windows"
}

// FileName get from Data structs member
func (d Data) FileName() string {
	return d.filename
}

// FullFileName get from Data structs member
func (d Data) FullFileName() string {
	return d.fullfilename
}

// FileSize get from Data structs member
func (d Data) FileSize() uint {
	return d.filesize
}

// DirName get from Data structs member
func (d Data) DirName() string {
	return d.dirname
}

// SetFileSize set to Data structs member
func (d *Data) SetFileSize(size uint) {
	d.filesize = size
}

// SetFullFileName set to Data structs member
func (d *Data) SetFullFileName(directory, filename string) {
	if directory == "" {
		d.fullfilename = fmt.Sprintf("%s", filename)
	} else {
		d.fullfilename = fmt.Sprintf("%s/%s", directory, filename)
	}
}

// SetFileName set to Data structs member
func (d *Data) SetFileName(filename string) {
	d.filename = filename
}

// URLFileName set to Data structs member using url
func (d *Data) URLFileName(targetDir, url string) string {
	token := strings.Split(url, "/")

	// get of filename from url
	var original string
	for i := 1; original == ""; i++ {
		original = token[len(token)-i]
	}

	filename := original

	// create unique filename
	for i := 1; true; i++ {
		var filePath string
		if targetDir == "" {
			filePath = filename
		} else {
			filePath = fmt.Sprintf("%s/%s", targetDir, filename)
		}

		if _, err := os.Stat(filePath); err == nil {
			filename = fmt.Sprintf("%s-%d", original, i)
		} else {
			break
		}
	}

	return filename
}

// SetDirName set to Data structs member
func (d *Data) SetDirName(path, filename string, procs int) {
	if path == "" {
		d.dirname = fmt.Sprintf("_%s.%d", filename, procs)
	} else {
		d.dirname = fmt.Sprintf("%s/_%s.%d", path, filename, procs)
	}

}

func (d Data) freeSpace() (freespace uint) {

	if isDos() {
		freespace = uint(du.NewDiskUsage("C:\\").Free())
	} else {
		freespace = uint(du.NewDiskUsage("/").Free())
	}

	return
}

// IsFree is check your disk space for size needed to download  检查下载空间是否足够
func (d Data) IsFree(split uint) error {
	want := d.filesize + split
	if d.freeSpace() < want {
		return errors.Errorf("there is not sufficient free space in a disk")
	}

	return nil
}

// Progress In order to confirm the degree of progress
func (d Data) Progress(dirname string) (int64, error) {
	return subDirsize(dirname)
}

func subDirsize(dirname string) (int64, error) {
	var size int64
	err := filepath.Walk(dirname, func(_ string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})

	return size, err
}

// MakeRange will return Range struct to download function
func (d *Data) MakeRange(i, split, procs uint) Range {
	low := split * i
	high := low + split - 1
	if i == procs-1 {
		high = d.FileSize()
	}

	return Range{
		low:    low,
		high:   high,
		worker: i,
	}
}

// ProgressBar is to show progressbar
func (d Data) ProgressBar(ctx context.Context) error {
	filesize := int64(d.filesize)
	dirname := d.dirname

	bar := pb.New64(filesize)
	bar.Start()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			size, err := d.Progress(dirname)
			if err != nil {
				return errors.Wrap(err, "failed to get directory size")
			}

			if size < filesize {
				bar.Set64(size)
			} else {
				bar.Set64(filesize)
				bar.Finish()
				return nil
			}

			// To save cpu resource
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// BindwithFiles function for file binding after split download
func (d *Data) BindwithFiles(procs int) error {

	fmt.Println("\nbinding with files...")

	filesize := d.filesize
	filename := d.filename
	dirname := d.dirname

	fh, err := os.Create(d.fullfilename)
	if err != nil {
		return errors.Wrap(err, "failed to create a file in download location")
	}
	defer fh.Close()

	bar := pb.New64(int64(filesize))
	bar.Start()

	var f string
	for i := 0; i < procs; i++ {
		f = fmt.Sprintf("%s/%s.%d.%d", dirname, filename, procs, i)
		subfp, err := os.Open(f)
		if err != nil {
			return errors.Wrap(err, "failed to open "+f+" in download location")
		}

		proxy := bar.NewProxyReader(subfp)
		io.Copy(fh, proxy)

		// Not use defer
		subfp.Close()

		// remove a file in download location for join
		if err := os.Remove(f); err != nil {
			return errors.Wrap(err, "failed to remove a file in download location")
		}
	}

	bar.Finish()

	// remove download location
	// RemoveAll reason: will create .DS_Store in download location if execute on mac
	if err := os.RemoveAll(dirname); err != nil {
		return errors.Wrap(err, "failed to remove download location")
	}

	fmt.Println("Complete")

	return nil
}
