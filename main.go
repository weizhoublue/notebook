package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
	}
}
