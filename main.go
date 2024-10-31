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

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/view/", viewHandler)
	http.HandleFunc("/create", createHandler)
	http.HandleFunc("/delete/", deleteHandler)
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
	log.Println("Handling index request")
	files, err := filepath.Glob("data/*.txt")
	if err != nil {
		log.Printf("Error globbing files: %v", err)
		http.Error(w, "Error reading notes", http.StatusInternalServerError)
		return
	}
	notes := make([]string, len(files))
	for i, file := range files {
		notes[i] = strings.TrimSuffix(filepath.Base(file), ".txt")
	}
	log.Printf("Found notes: %v", notes)
	templates.ExecuteTemplate(w, "index.html", notes)
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Path[len("/view/"):]
	log.Printf("Viewing note: %s", title)
	body, err := os.ReadFile("data/" + title + ".txt")
	if err != nil {
		log.Printf("Error reading file %s: %v", title, err)
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
		isEdit := r.FormValue("isEdit") == "true"

		log.Printf("Attempting to %s note with title: %s", map[bool]string{true: "edit", false: "create"}[isEdit], title)

		if strings.ContainsAny(title, `/\:*?"<>|`) {
			log.Printf("Invalid note title: %s", title)
			http.Error(w, "无效的记事本标题", http.StatusBadRequest)
			return
		}

		if _, err := os.Stat("data"); os.IsNotExist(err) {
			log.Println("Data directory does not exist, creating...")
			err = os.Mkdir("data", 0755)
			if err != nil {
				log.Printf("Error creating data directory: %v", err)
				http.Error(w, "无法创建数据目录", http.StatusInternalServerError)
				return
			}
		}

		filePath := "data/" + title + ".txt"
		if !isEdit {
			if _, err := os.Stat(filePath); err == nil {
				log.Printf("Note with title %s already exists", title)
				http.Error(w, "该标题的记事本已存在", http.StatusConflict)
				return
			} else if !os.IsNotExist(err) {
				log.Printf("Error checking file %s: %v", filePath, err)
				http.Error(w, "无法检查文件", http.StatusInternalServerError)
				return
			}
		}

		err := os.WriteFile(filePath, []byte(body), 0644)
		if err != nil {
			log.Printf("Error writing file %s: %v", filePath, err)
			http.Error(w, "无法保存记事本", http.StatusInternalServerError)
			return
		}

		log.Printf("Note with title %s %s successfully", title, map[bool]string{true: "edited", false: "created"}[isEdit])
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Error(w, "无效的请求方法", http.StatusMethodNotAllowed)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Path[len("/delete/"):]
	err := os.Remove("data/" + title + ".txt")
	if err != nil {
		log.Printf("Error deleting file %s: %v", title, err)
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.FormValue("query")
	files, err := filepath.Glob("data/*.txt")
	if err != nil {
		log.Printf("Error globbing files: %v", err)
		http.Error(w, "Error searching notes", http.StatusInternalServerError)
		return
	}
	var results []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			log.Printf("Error reading file %s: %v", file, err)
			continue
		}
		if strings.Contains(string(content), query) {
			results = append(results, strings.TrimSuffix(filepath.Base(file), ".txt"))
		}
	}
	log.Printf("Search results: %v", results)
	templates.ExecuteTemplate(w, "index.html", results)
}
