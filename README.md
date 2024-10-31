# Notebook

## 项目简介

这是一个简单的记事本管理应用程序，使用 Go 语言编写，支持创建、编辑、删除和备份记事本。

## 使用 `go:embed` 嵌入模板文件

在本项目中，我们使用了 Go 1.16 引入的 `go:embed` 特性来嵌入 HTML 模板文件。这使得在编译时将模板文件嵌入到二进制文件中，确保在任何目录下运行时都能正确加载模板。

### 如何使用

1. **导入 `embed` 包**：
   在 `notebook.go` 文件中，导入 `embed` 包以使用 `go:embed` 特性。
   ```go
   import (
       "embed"
       _ "embed"
   )
   ```

2. **使用 `go:embed` 指令嵌入模板文件**：
   使用 `go:embed` 指令将 `templates` 文件夹中的所有 HTML 文件嵌入到程序中。
   ```go
   //go:embed templates/*.html
   var templateFiles embed.FS
   ```

3. **解析嵌入的模板文件**：
   使用 `template.ParseFS` 方法从嵌入的文件系统中解析模板。
   ```go
   var templates = template.Must(template.ParseFS(templateFiles, "templates/*.html"))
   ```

### 好处

- **便携性**：生成的二进制文件包含所有必要的模板文件，无需在运行时依赖外部文件。
- **简化部署**：无需在部署时复制模板文件，只需部署单个二进制文件即可。

## 数据存储目录

默认情况下，数据和备份将存储在用户的 `Documents` 目录下的 `notebookData` 文件夹中：
- 数据目录：`~/Documents/notebookData/data`
- 备份目录：`~/Documents/notebookData/backup`

## 运行项目

确保你已经安装了 Go 1.16 或更高版本。

1. 克隆项目到本地：
   ```bash
   git clone https://github.com/yourusername/notebook.git
   cd notebook
   ```

2. 构建项目：
   ```bash
   go build -o notebook
   ```

3. 运行项目：
   ```bash
   ./notebook
   ```

4. 打开浏览器访问：
   ```
   http://localhost:8080
   ```

## 目录结构

- `notebook.go`：主程序文件，包含所有的业务逻辑。
- `templates/`：HTML 模板文件，已通过 `go:embed` 嵌入到程序中。

## 贡献

欢迎提交问题和请求合并。请确保在提交之前运行所有测试。