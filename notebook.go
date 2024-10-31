package main

import (
	"archive/zip"
	"embed"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

//go:embed templates/*.html
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "templates/*.html"))

var (
	workerHomePath string
	dataDir        string
	backupDir      string
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 获取当前用户的 home 目录
	usr, err := user.Current()
	if err != nil {
		log.Fatalf("Failed to get current user: %v", err)
	}
	workerHomePath = usr.HomeDir

	// 设置数据目录和备份目录
	dataDir = filepath.Join(workerHomePath, "Documents", "notebookData", "data")
	backupDir = filepath.Join(workerHomePath, "Documents", "notebookData", "backup")
}

func logWithFuncName(message string) {
	pc, _, _, _ := runtime.Caller(1)
	funcName := runtime.FuncForPC(pc).Name()
	log.Printf("[%s] %s", funcName, message)
}

func main() {
	// 打印程序工作的根目录
	log.Printf("root directory: %s", workerHomePath)
	log.Printf("data directory: %s", dataDir)
	log.Printf("backup directory: %s", backupDir)

	// 确保数据目录和备份目录存在
	ensureDirExists(dataDir)
	ensureDirExists(backupDir)

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/view/", viewHandler)
	http.HandleFunc("/create", createHandler)
	http.HandleFunc("/edit", editHandler)
	http.HandleFunc("/delete/", deleteHandler)
	http.HandleFunc("/delete-all", deleteAllHandler)
	http.HandleFunc("/create-dir", createDirHandler)
	http.HandleFunc("/delete-dir", deleteDirHandler)
	http.HandleFunc("/backup-count", backupCountHandler)
	http.HandleFunc("/list-dirs", listDirsHandler)
	http.HandleFunc("/notes", listNotesHandler)

	go func() {
		fmt.Println("Server started at :8080")
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	openBrowser("http://localhost:8080")

	select {}
}

func ensureDirExists(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}
}

func backupData(dirName string) {
	logWithFuncName(fmt.Sprintf("Starting backup for directory: %s", dirName))
	timestamp := time.Now().Format("20060102_150405")
	backupDir := filepath.Join(backupDir, dirName)
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s.zip", timestamp))

	err := os.MkdirAll(backupDir, 0755)
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error creating backup directory: %v", err))
		return
	}

	err = zipDirectory(filepath.Join(dataDir, dirName), backupFile)
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error creating backup zip: %v", err))
		return
	}

	logWithFuncName(fmt.Sprintf("Backup created successfully: %s", backupFile))
	manageBackups(backupDir)
}

func zipDirectory(source, target string) error {
	logWithFuncName(fmt.Sprintf("Zipping directory: %s to %s", source, target))
	zipfile, err := os.Create(target)
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error creating zip file: %v", err))
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error walking path %s: %v", path, err))
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error creating zip header for %s: %v", path, err))
			return err
		}

		header.Name = strings.TrimPrefix(path, filepath.Dir(source)+"/")
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error creating zip writer for %s: %v", path, err))
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error opening file %s: %v", path, err))
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error copying file %s to zip: %v", path, err))
			}
		}
		return err
	})

	if err != nil {
		logWithFuncName(fmt.Sprintf("Error zipping directory %s: %v", source, err))
	}
	return err
}

func manageBackups(backupDir string) {
	logWithFuncName(fmt.Sprintf("Managing backups in directory: %s", backupDir))
	files, err := filepath.Glob(filepath.Join(backupDir, "*.zip"))
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error listing backup files: %v", err))
		return
	}

	logWithFuncName(fmt.Sprintf("Found %d backup files", len(files)))
	if len(files) > 50 {
		sort.Strings(files)
		for _, file := range files[:len(files)-50] {
			err := os.Remove(file)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error removing old backup file %s: %v", file, err))
			} else {
				logWithFuncName(fmt.Sprintf("Removed old backup file: %s", file))
			}
		}
	}
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		dirName := strings.TrimSpace(r.FormValue("dirName"))
		title := strings.TrimSpace(r.FormValue("title"))
		body := r.FormValue("body")

		logWithFuncName(fmt.Sprintf("Received request to create note with title: %s in directory: %s", title, dirName))

		if strings.ContainsAny(title, `/\:*?"<>|`) {
			logWithFuncName(fmt.Sprintf("Invalid note title: %s", title))
			http.Error(w, "无效的记事本标题", http.StatusBadRequest)
			return
		}

		dirPath := filepath.Join(dataDir, dirName)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			logWithFuncName(fmt.Sprintf("Directory %s does not exist", dirPath))
			http.Error(w, "目录不存在", http.StatusNotFound)
			return
		}

		filePath := filepath.Join(dirPath, title+".txt")
		if _, err := os.Stat(filePath); err == nil {
			logWithFuncName(fmt.Sprintf("Note with title %s already exists in directory %s", title, dirName))
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

		logWithFuncName(fmt.Sprintf("Note with title %s created successfully in directory %s", title, dirName))
		backupData(dirName) // 备份数据
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
}

func editHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		dirName := strings.TrimSpace(r.FormValue("dirName"))
		oldTitle := strings.TrimSpace(r.FormValue("oldTitle"))
		newTitle := strings.TrimSpace(r.FormValue("title"))
		body := r.FormValue("body")

		logWithFuncName(fmt.Sprintf("Editing note from title: %s to new title: %s in directory: %s", oldTitle, newTitle, dirName))

		if strings.ContainsAny(newTitle, `/\:*?"<>|`) {
			logWithFuncName(fmt.Sprintf("Invalid new note title: %s", newTitle))
			http.Error(w, "无效的新记事本标题", http.StatusBadRequest)
			return
		}

		dirPath := filepath.Join(dataDir, dirName)
		oldFilePath := filepath.Join(dirPath, oldTitle+".txt")
		newFilePath := filepath.Join(dirPath, newTitle+".txt")

		if oldTitle == newTitle {
			err := os.WriteFile(oldFilePath, []byte(body), 0644)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error writing file %s: %v", oldFilePath, err))
				http.Error(w, "无法保存记事本", http.StatusInternalServerError)
				return
			}
			logWithFuncName(fmt.Sprintf("Note with title %s edited successfully in directory %s", newTitle, dirName))
			backupData(dirName) // 备份数据
			w.WriteHeader(http.StatusOK)
			return
		}

		if _, err := os.Stat(newFilePath); err == nil {
			logWithFuncName(fmt.Sprintf("Note with new title %s already exists in directory %s", newTitle, dirName))
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

		logWithFuncName(fmt.Sprintf("Note with title %s edited successfully in directory %s", newTitle, dirName))
		backupData(dirName) // 备份数据
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
}

func listNotesHandler(w http.ResponseWriter, r *http.Request) {
	dirName := r.URL.Query().Get("dirName")
	dirPath := filepath.Join(dataDir, dirName)

	files, err := filepath.Glob(filepath.Join(dirPath, "*.txt"))
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error listing notes in directory %s: %v", dirName, err))
		http.Error(w, "无法获取记事本列表", http.StatusInternalServerError)
		return
	}

	var notes []string
	for _, file := range files {
		notes = append(notes, strings.TrimSuffix(filepath.Base(file), ".txt"))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notes)
}

func listDirsHandler(w http.ResponseWriter, r *http.Request) {
	dirs, err := filepath.Glob(dataDir + "/*")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error listing directories: %v", err))
		http.Error(w, "无法获取目录列表", http.StatusInternalServerError)
		return
	}

	var directories []string
	for _, dir := range dirs {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			directories = append(directories, filepath.Base(dir))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(directories)
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
	dirName := r.URL.Query().Get("dirName")
	title := r.URL.Path[len("/view/"):]
	filePath := filepath.Join(dataDir, dirName, title+".txt")

	body, err := os.ReadFile(filePath)
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error reading file %s: %v", filePath, err))
		http.NotFound(w, r)
		return
	}

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

func backupCountHandler(w http.ResponseWriter, r *http.Request) {
	logWithFuncName("Handling backup count request")
	backupFiles, err := filepath.Glob("backup/*/*.zip")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error listing backup files: %v", err))
		http.Error(w, "无法获取备份数量", http.StatusInternalServerError)
		return
	}

	count := len(backupFiles)
	logWithFuncName(fmt.Sprintf("Found %d backup files", count))
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"count": %d}`, count)))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	logWithFuncName("Handling index request")
	dirs, err := filepath.Glob(dataDir + "/*")
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error globbing directories: %v", err))
		http.Error(w, "Error reading directories", http.StatusInternalServerError)
		return
	}

	var directories []string
	for _, dir := range dirs {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			directories = append(directories, filepath.Base(dir))
		}
	}

	logWithFuncName(fmt.Sprintf("Found directories: %v", directories))
	templates.ExecuteTemplate(w, "index.html", directories)
}

func createDirHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		dirName := strings.TrimSpace(r.FormValue("dirName"))
		if dirName == "" {
			http.Error(w, "目录名不能为空", http.StatusBadRequest)
			return
		}

		dirPath := filepath.Join(dataDir, dirName)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			err = os.Mkdir(dirPath, 0755)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error creating directory %s: %v", dirPath, err))
				http.Error(w, "无法创建目录", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "目录已存在", http.StatusConflict)
			return
		}

		logWithFuncName(fmt.Sprintf("Directory %s created successfully", dirName))
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
	}
}

func deleteDirHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		dirName := strings.TrimSpace(r.FormValue("dirName"))
		if dirName == "" {
			http.Error(w, "目录名不能为空", http.StatusBadRequest)
			return
		}

		dirPath := filepath.Join(dataDir, dirName)
		if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
			err = os.RemoveAll(dirPath)
			if err != nil {
				logWithFuncName(fmt.Sprintf("Error deleting directory %s: %v", dirPath, err))
				http.Error(w, "无法删除目录", http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "目录不存在", http.StatusNotFound)
			return
		}

		logWithFuncName(fmt.Sprintf("Directory %s deleted successfully", dirName))
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
	}
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	dirName := r.URL.Query().Get("dirName")
	title := r.URL.Path[len("/delete/"):]
	filePath := filepath.Join(dataDir, dirName, title+".txt")

	err := os.Remove(filePath)
	if err != nil {
		logWithFuncName(fmt.Sprintf("Error deleting file %s: %v", filePath, err))
		http.Error(w, "无法删除记事本", http.StatusInternalServerError)
		return
	}

	logWithFuncName(fmt.Sprintf("Note with title %s deleted successfully from directory %s", title, dirName))
	http.Redirect(w, r, "/", http.StatusFound)
}

func deleteAllHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		dirName := strings.TrimSpace(r.FormValue("dirName"))
		dirPath := filepath.Join(dataDir, dirName)

		files, err := filepath.Glob(filepath.Join(dirPath, "*.txt"))
		if err != nil {
			logWithFuncName(fmt.Sprintf("Error globbing files in directory %s: %v", dirName, err))
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

		logWithFuncName(fmt.Sprintf("All notes deleted successfully from directory %s", dirName))
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
	}
}
