// Licensed to Apache Software Foundation (ASF) under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Apache Software Foundation (ASF) licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Package fs (file system) is an independent component to operate file and directory.
package fs

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/apache/skywalking-banyandb/pkg/logger"
)

// localFileSystem implements the File System interface.
type localFileSystem struct {
	logger *logger.Logger
}

// LocalFile implements the File interface.
type LocalFile struct {
	file *os.File
}

// NewLocalFileSystem is used to create the Local File system.
func NewLocalFileSystem() FileSystem {
	return &localFileSystem{
		logger: logger.GetLogger(moduleName),
	}
}

// NewLocalFileSystemWithLogger is used to create the Local File system with logger.
func NewLocalFileSystemWithLogger(parent *logger.Logger) FileSystem {
	return &localFileSystem{
		logger: parent.Named(moduleName),
	}
}

func readErrorHandle(operation string, err error, name string, size int) (int, error) {
	switch {
	case errors.Is(err, io.EOF):
		return size, err
	case os.IsNotExist(err):
		return size, &FileSystemError{
			Code:    IsNotExistError,
			Message: fmt.Sprintf("%s failed, file is not exist, file name: %s, error message: %s", operation, name, err),
		}
	case os.IsPermission(err):
		return size, &FileSystemError{
			Code:    permissionError,
			Message: fmt.Sprintf("%s failed, there is not enough permission, file name: %s, error message: %s", operation, name, err),
		}
	default:
		return size, &FileSystemError{
			Code:    readError,
			Message: fmt.Sprintf("%s failed, read file error, file name: %s, read file size: %d, error message: %s", operation, name, size, err),
		}
	}
}

func (fs *localFileSystem) MkdirIfNotExist(path string, permission Mode) {
	if fs.pathExist(path) {
		return
	}
	fs.mkdir(path, permission)
}

func (fs *localFileSystem) MkdirPanicIfExist(path string, permission Mode) {
	if fs.pathExist(path) {
		fs.logger.Panic().Str("path", path).Msg("directory is exist")
	}
	fs.mkdir(path, permission)
}

func (fs *localFileSystem) mkdir(path string, permission Mode) {
	if err := os.MkdirAll(path, os.FileMode(permission)); err != nil {
		fs.logger.Panic().Str("path", path).Err(err).Msg("failed to create directory")
	}
	parentDirPath := filepath.Dir(path)
	fs.syncPath(parentDirPath)
}

func (fs *localFileSystem) pathExist(path string) bool {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		fs.logger.Panic().Str("path", path).Err(err).Msg("failed to stat path")
	}
	return true
}

func (fs *localFileSystem) syncPath(path string) {
	d, err := os.Open(path)
	if err != nil {
		fs.logger.Panic().Str("path", path).Err(err).Msg("failed to open directory")
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		fs.logger.Panic().Str("path", path).Err(err).Msg("failed to sync directory")
	}
	if err := d.Close(); err != nil {
		fs.logger.Panic().Str("path", path).Err(err).Msg("ailed to sync directory")
	}
}

func (fs *localFileSystem) ReadDir(dirname string) []DirEntry {
	des, err := os.ReadDir(dirname)
	if err != nil {
		fs.logger.Panic().Str("dirname", dirname).Err(err).Msg("failed to read directory")
	}
	result := make([]DirEntry, len(des))
	for i, de := range des {
		result[i] = DirEntry(de)
	}
	return result
}

// CreateFile is used to create and open the file by specified name and mode.
func (fs *localFileSystem) CreateFile(name string, permission Mode) (File, error) {
	file, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(permission))
	switch {
	case err == nil:
		return &LocalFile{
			file: file,
		}, nil
	case os.IsExist(err):
		return nil, &FileSystemError{
			Code:    isExistError,
			Message: fmt.Sprintf("File is exist, file name: %s,error message: %s", name, err),
		}
	case os.IsPermission(err):
		return nil, &FileSystemError{
			Code:    permissionError,
			Message: fmt.Sprintf("There is not enough permission, file name: %s, permission: %d,error message: %s", name, permission, err),
		}
	default:
		return nil, &FileSystemError{
			Code:    otherError,
			Message: fmt.Sprintf("Create file return error, file name: %s,error message: %s", name, err),
		}
	}
}

func (fs *localFileSystem) OpenFile(name string) (File, error) {
	file, err := os.Open(name)
	switch {
	case err == nil:
		return &LocalFile{
			file: file,
		}, nil
	case os.IsNotExist(err):
		return nil, &FileSystemError{
			Code:    IsNotExistError,
			Message: fmt.Sprintf("File is not exist, file name: %s,error message: %s", name, err),
		}
	case os.IsPermission(err):
		return nil, &FileSystemError{
			Code:    permissionError,
			Message: fmt.Sprintf("There is not enough permission, file name: %s, error message: %s", name, err),
		}
	default:
		return nil, &FileSystemError{
			Code:    otherError,
			Message: fmt.Sprintf("Create file return error, file name: %s,error message: %s", name, err),
		}
	}
}

// Write flushes all data to one file.
func (fs *localFileSystem) Write(buffer []byte, name string, permission Mode) (int, error) {
	file, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(permission))
	if err != nil {
		switch {
		case os.IsExist(err):
			return 0, &FileSystemError{
				Code:    isExistError,
				Message: fmt.Sprintf("File is exist, file name: %s,error message: %s", name, err),
			}
		case os.IsPermission(err):
			return 0, &FileSystemError{
				Code:    permissionError,
				Message: fmt.Sprintf("There is not enough permission, file name: %s, permission: %d,error message: %s", name, permission, err),
			}
		default:
			return 0, &FileSystemError{
				Code:    otherError,
				Message: fmt.Sprintf("Create file return error, file name: %s,error message: %s", name, err),
			}
		}
	}
	defer file.Close()

	size, err := file.Write(buffer)
	if err != nil {
		return size, &FileSystemError{
			Code:    flushError,
			Message: fmt.Sprintf("Flush file return error, file name: %s,error message: %s", name, err),
		}
	}

	return size, nil
}

// Read is used to read the entire file using streaming read.
func (fs *localFileSystem) Read(name string) ([]byte, error) {
	data, err := os.ReadFile(name)
	switch {
	case err == nil:
		return data, nil
	case os.IsNotExist(err):
		return data, &FileSystemError{
			Code:    IsNotExistError,
			Message: fmt.Sprintf("File is not exist, file name: %s, error message: %s", name, err),
		}
	case os.IsPermission(err):
		return data, &FileSystemError{
			Code:    permissionError,
			Message: fmt.Sprintf("There is not enough permission, file name: %s, error message: %s", name, err),
		}
	default:
		return data, &FileSystemError{
			Code:    otherError,
			Message: fmt.Sprintf("Read file error, file name: %s, error message: %s", name, err),
		}
	}
}

// DeleteFile is used to delete the file.
func (fs *localFileSystem) DeleteFile(name string) error {
	err := os.Remove(name)
	switch {
	case err == nil:
		return nil
	case os.IsNotExist(err):
		return &FileSystemError{
			Code:    IsNotExistError,
			Message: fmt.Sprintf("File is not exist, file name: %s, error message: %s", name, err),
		}
	case os.IsPermission(err):
		return &FileSystemError{
			Code:    permissionError,
			Message: fmt.Sprintf("There is not enough permission, file name: %s, error message: %s", name, err),
		}
	default:
		return &FileSystemError{
			Code:    otherError,
			Message: fmt.Sprintf("Delete file error, file name: %s, error message: %s", name, err),
		}
	}
}

func (fs *localFileSystem) MustRMAll(path string) {
	if err := os.RemoveAll(path); err == nil {
		return
	}
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		if err := os.RemoveAll(path); err == nil {
			return
		}
	}
	fs.logger.Panic().Str("path", path).Msg("failed to remove all files under path")
}

// Write adds new data to the end of a file.
func (file *LocalFile) Write(buffer []byte) (int, error) {
	size, err := file.file.Write(buffer)
	switch {
	case err == nil:
		return size, nil
	case os.IsNotExist(err):
		return size, &FileSystemError{
			Code:    IsNotExistError,
			Message: fmt.Sprintf("File is not exist, file name: %s, error message: %s", file.file.Name(), err),
		}
	case os.IsPermission(err):
		return size, &FileSystemError{
			Code:    permissionError,
			Message: fmt.Sprintf("There is not enough permission, file name: %s, error message: %s", file.file.Name(), err),
		}
	default:
		// include io.ErrShortWrite
		return size, &FileSystemError{
			Code:    writeError,
			Message: fmt.Sprintf("Write file error, file name: %s, error message: %s", file.file.Name(), err),
		}
	}
}

// Writev supports appending consecutive buffers to the end of the file.
// TODO: Optimizing under Linux.
func (file *LocalFile) Writev(iov *[][]byte) (int, error) {
	var size int
	for _, buffer := range *iov {
		wsize, err := file.file.Write(buffer)
		switch {
		case err == nil:
			size += wsize
		case os.IsNotExist(err):
			return size, &FileSystemError{
				Code:    IsNotExistError,
				Message: fmt.Sprintf("File is not exist, file name: %s, error message: %s", file.file.Name(), err),
			}
		case os.IsPermission(err):
			return size, &FileSystemError{
				Code:    permissionError,
				Message: fmt.Sprintf("There is not enough permission, file name: %s, error message: %s", file.file.Name(), err),
			}
		default:
			// include io.ErrShortWrite
			return size, &FileSystemError{
				Code:    writeError,
				Message: fmt.Sprintf("Write file error, file name: %s, error message: %s", file.file.Name(), err),
			}
		}
	}
	return size, nil
}

// Read is used to read a specified location of file.
func (file *LocalFile) Read(offset int64, buffer []byte) (int, error) {
	rsize, err := file.file.ReadAt(buffer, offset)
	if err != nil {
		return readErrorHandle("Read operation", err, file.file.Name(), rsize)
	}

	return rsize, nil
}

// Readv is used to read contiguous regions of a file and disperse them into discontinuous buffers.
// TODO: Optimizing under Linux.
func (file *LocalFile) Readv(offset int64, iov *[][]byte) (int, error) {
	var size int
	for _, buffer := range *iov {
		rsize, err := file.file.ReadAt(buffer, offset)
		if err != nil {
			return readErrorHandle("Readv operation", err, file.file.Name(), rsize)
		}
		size += rsize
		offset += int64(rsize)
	}

	return size, nil
}

// StreamRead is used to read the entire file using streaming read.
func (file *LocalFile) StreamRead(buffer []byte) (*Iter, error) {
	reader := bufio.NewReader(file.file)
	return &Iter{reader: reader, buffer: buffer, fileName: file.file.Name()}, nil
}

// Size is used to get the file written data's size and return an error if the file does not exist. The unit of file size is Byte.
func (file *LocalFile) Size() (int64, error) {
	fileInfo, err := os.Stat(file.file.Name())
	if err != nil {
		if os.IsNotExist(err) {
			return -1, &FileSystemError{
				Code:    IsNotExistError,
				Message: fmt.Sprintf("File is not exist, file name: %s, error message: %s", file.file.Name(), err),
			}
		} else if os.IsPermission(err) {
			return -1, &FileSystemError{
				Code:    permissionError,
				Message: fmt.Sprintf("There is not enough permission, file name: %s, error message: %s", file.file.Name(), err),
			}
		}
		return -1, &FileSystemError{
			Code:    otherError,
			Message: fmt.Sprintf("Get file size error, file name: %s, error message: %s", file.file.Name(), err),
		}
	}
	return fileInfo.Size(), nil
}

// Path returns the absolute path of the file.
func (file *LocalFile) Path() string {
	return file.file.Name()
}

// Close is used to close File.
func (file *LocalFile) Close() error {
	if err := file.file.Sync(); err != nil {
		return &FileSystemError{
			Code:    closeError,
			Message: fmt.Sprintf("Close File error, directory name: %s, error message: %s", file.file.Name(), err),
		}
	}

	if err := file.file.Close(); err != nil {
		return &FileSystemError{
			Code:    closeError,
			Message: fmt.Sprintf("Close File error, directory name: %s, error message: %s", file.file.Name(), err),
		}
	}
	return nil
}

// Next is used to obtain the next data of stream.
func (iter *Iter) Next() (int, error) {
	rsize, err := iter.reader.Read(iter.buffer)
	if err != nil {
		return readErrorHandle("ReadStream operation", err, iter.fileName, rsize)
	}
	return rsize, nil
}
