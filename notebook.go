package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

var templates = template.Must(template.ParseGlob("templates/*.html"))

func init() {
	// 设置日志格式，包含日期、时间、文件名和行号
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func logWithFuncName(message string) {
	pc, _, _, _ := runtime.Caller(1)
	funcName := runtime.FuncForPC(pc).Name()
	log.Printf("[%s] %s", funcName, message)
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/view/", viewHandler)
	http.HandleFunc("/create", createHandler)
	http.HandleFunc("/edit", editHandler)
	http.HandleFunc("/delete/", deleteHandler)
	http.HandleFunc("/delete-all", deleteAllHandler)
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/backup-count", backupCountHandler)

	// 启动服务器
	go func() {
		fmt.Println("Server started at :8080")
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	// 打开浏览器
	openBrowser("http://localhost:8080")

	// 阻止主协程退出
	select {}
}

func backupData() {
	// 获取当前时间戳
	timestamp := time.Now().Format("20060102_150405")
	backupDir := "backup/" + timestamp

	// 管理备份数量
	manageBackups()

	// 创建备份目录
	err := os.MkdirAll(backupDir, 0755)
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error creating backup directory: %v", err))
		return
	}

	// 复制 data 目录中的文件到备份目录
	files, err := filepath.Glob("data/*.txt")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error globbing files for backup: %v", err))
		return
	}

	for _, file := range files {
		// 获取文件名
		fileName := filepath.Base(file)
		backupFilePath := filepath.Join(backupDir, fileName)

		// 复制文件
		input, err := os.ReadFile(file)
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error reading file %s for backup: %v", file, err))
			continue
		}

		err = os.WriteFile(backupFilePath, input, 0644)
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error writing backup file %s: %v", backupFilePath, err))
		}
	}

	// 压缩整个备份目录
	tarGzPath := backupDir + ".tar.gz"
	err = compressDirectory(backupDir, tarGzPath)
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error compressing backup directory %s: %v", backupDir, err))
		return
	}

	// 删除未压缩的备份目录
	err = os.RemoveAll(backupDir)
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error removing uncompressed backup directory %s: %v", backupDir, err))
	}

	logWithFuncName(fmt.Sprintf("Data backup completed at %s", timestamp))
}

func compressDirectory(source, target string) error {
	tarFile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	gzipWriter := gzip.NewWriter(tarFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.Walk(source, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create a new tar header from the file info
		header, err := tar.FileInfoHeader(fi, fi.Name())
		if err != nil {
			return err
		}

		// Update the name to maintain the directory structure
		header.Name, err = filepath.Rel(filepath.Dir(source), file)
		if err != nil {
			return err
		}

		// Write the header to the tar file
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// If the file is a directory, we don't need to copy the file content
		if fi.IsDir() {
			return nil
		}

		// Open the file for reading
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		// Copy the file content to the tar file
		_, err = io.Copy(tarWriter, f)
		return err
	})
}

func manageBackups() {
	backupFiles, err := filepath.Glob("backup/*.tar.gz")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error listing backup files: %v", err))
		return
	}

	// 如果备份数量超过 50，删除最老的备份
	if len(backupFiles) > 50 {
		sort.Strings(backupFiles) // 按时间戳排序
		for _, file := range backupFiles[:len(backupFiles)-50] {
			err := os.Remove(file)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error removing old backup file %s: %v", file, err))
			} else {
				logWithFuncName(fmt.Sprintf("Removed old backup file %s", file))
			}
		}
	}
}

func openBrowser(url string) {
	var err error

	switch os := runtime.GOOS; os {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	if err != nil {
		log.Fatalf("Failed to open browser: %v", err)
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	logWithFuncName("Handling index request")
	files, err := filepath.Glob("data/*.txt")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error globbing files: %v", err))
		http.Error(w, "Error reading notes", http.StatusInternalServerError)
		return
	}
	notes := make([]string, len(files))
	for i, file := range files {
		notes[i] = strings.TrimSuffix(filepath.Base(file), ".txt")
	}
	logWithFuncName(fmt.Sprintf("Found notes: %v", notes))
	templates.ExecuteTemplate(w, "index.html", notes)
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Path[len("/view/"):]
	logWithFuncName(fmt.Sprintf("Viewing note: %s", title))
	body, err := os.ReadFile("data/" + title + ".txt")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error reading file %s: %v", title, err))
		http.NotFound(w, r)
		return
	}

	// 检查请求头是否要求纯文本
	if r.Header.Get("Accept") == "text/plain" {
		w.Header().Set("Content-Type", "text/plain")
		w.Write(body)
		return
	}

	templates.ExecuteTemplate(w, "view.html", struct {
		Title string
		Body  string
	}{Title: title, Body: string(body)})
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		title := strings.TrimSpace(r.FormValue("title"))
		body := r.FormValue("body")

		logWithFuncName(fmt.Sprintf("Creating note with title: %s", title))

		if strings.ContainsAny(title, `/\:*?"<>|`) {
			logWithFuncName(fmt.Sprintf("Invalid note title: %s", title))
			http.Error(w, "无效的记事本标题", http.StatusBadRequest)
			return
		}

		if _, err := os.Stat("data"); os.IsNotExist(err) {
			logWithFuncName("Data directory does not exist, creating...")
			err = os.Mkdir("data", 0755)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error creating data directory: %v", err))
				http.Error(w, "无法创建数据目录", http.StatusInternalServerError)
				return
			}
		}

		filePath := "data/" + title + ".txt"
		logWithFuncName(fmt.Sprintf("File path for new note: %s", filePath))
		if _, err := os.Stat(filePath); err == nil {
			logWithFuncName(fmt.Sprintf("Note with title %s already exists", title))
			http.Error(w, "该标题的记事本已存在", http.StatusConflict)
			return
		} else if !os.IsNotExist(err) {
			logWithFuncName(fmt.Sprintf("Error checking file %s: %v", filePath, err))
			http.Error(w, "无法检查文件", http.StatusInternalServerError)
			return
		}

		err := os.WriteFile(filePath, []byte(body), 0644)
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error writing file %s: %v", filePath, err))
			http.Error(w, "无法保存记事本", http.StatusInternalServerError)
			return
		}

		logWithFuncName(fmt.Sprintf("Note with title %s created successfully", title))
		backupData() // 在创建后进行备份
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
}

func editHandler(w http.ResponseWriter, r *http.Request) {
	logWithFuncName(fmt.Sprintf("Received Request: %+v'", r))

	if r.Method == http.MethodPost {
		oldTitle := strings.TrimSpace(r.FormValue("oldTitle"))
		newTitle := strings.TrimSpace(r.FormValue("title"))
		body := r.FormValue("body")

		// 打印接收到的参数
		logWithFuncName(fmt.Sprintf("Received parameters - oldTitle: '%s', newTitle: '%s', body: '%s'", oldTitle, newTitle, body))

		logWithFuncName(fmt.Sprintf("Editing note from title: %s to new title: %s", oldTitle, newTitle))

		if strings.ContainsAny(newTitle, `/\:*?"<>|`) {
			logWithFuncName(fmt.Sprintf("Invalid new note title: %s", newTitle))
			http.Error(w, "无效的新记事本标题", http.StatusBadRequest)
			return
		}

		oldFilePath := "data/" + oldTitle + ".txt"
		newFilePath := "data/" + newTitle + ".txt"

		// 如果旧标题和新标题相同，直接更新内容
		if oldTitle == newTitle {
			err := os.WriteFile(oldFilePath, []byte(body), 0644)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error writing file %s: %v", oldFilePath, err))
				http.Error(w, "无法保存记事本", http.StatusInternalServerError)
				return
			}
			logWithFuncName(fmt.Sprintf("Note with title %s edited successfully", newTitle))
			backupData() // 在编辑后进行备份
			w.WriteHeader(http.StatusOK)
			return
		}

		if _, err := os.Stat(newFilePath); err == nil {
			logWithFuncName(fmt.Sprintf("Note with new title %s already exists", newTitle))
			http.Error(w, "该新标题的记事本已存在", http.StatusConflict)
			return
		} else if !os.IsNotExist(err) {
			logWithFuncName(fmt.Sprintf("Error checking file %s: %v", newFilePath, err))
			http.Error(w, "无法检查文件", http.StatusInternalServerError)
			return
		}

		err := os.WriteFile(newFilePath, []byte(body), 0644)
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error writing file %s: %v", newFilePath, err))
			http.Error(w, "无法保存记事本", http.StatusInternalServerError)
			return
		}

		err = os.Remove(oldFilePath)
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error deleting old file %s: %v", oldFilePath, err))
		}

		logWithFuncName(fmt.Sprintf("Note with title %s edited successfully", newTitle))
		backupData() // 在编辑后进行备份
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Path[len("/delete/"):]
	err := os.Remove("data/" + title + ".txt")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error deleting file %s: %v", title, err))
	}
	backupData() // 在删除后进行备份
	http.Redirect(w, r, "/", http.StatusFound)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.FormValue("query")
	files, err := filepath.Glob("data/*.txt")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error globbing files: %v", err))
		http.Error(w, "Error searching notes", http.StatusInternalServerError)
		return
	}
	var results []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error reading file %s: %v", file, err))
			continue
		}
		if strings.Contains(string(content), query) {
			results = append(results, strings.TrimSuffix(filepath.Base(file), ".txt"))
		}
	}
	logWithFuncName(fmt.Sprintf("Search results: %v", results))
	templates.ExecuteTemplate(w, "index.html", results)
}

func deleteAllHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		files, err := filepath.Glob("data/*.txt")
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error globbing files: %v", err))
			http.Error(w, "无法删除记事本", http.StatusInternalServerError)
			return
		}
		for _, file := range files {
			err := os.Remove(file)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error deleting file %s: %v", file, err))
				http.Error(w, "无法删除记事本", http.StatusInternalServerError)
				return
			}
		}
		logWithFuncName("All notes deleted successfully")
		backupData() // 在删除所有后进行备份
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
	}
}

func backupCountHandler(w http.ResponseWriter, r *http.Request) {
	backupFiles, err := filepath.Glob("backup/*.tar.gz")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error listing backup files: %v", err))
		http.Error(w, "无法获取备份数量", http.StatusInternalServerError)
		return
	}

	count := len(backupFiles)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"count": %d}`, count)))
}
