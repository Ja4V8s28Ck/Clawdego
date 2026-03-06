package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func main() {
	local := os.Getenv("local")

	err := godotenv.Load()
	if err != nil && local == "true" {
		log.Println("Error loading .env file")
	}
	var prompt string
	flag.StringVar(&prompt, "p", "", "Prompt to send to LLM")
	flag.Parse()
	var model string

	if prompt == "" {
		panic("Prompt must not be empty")
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	baseUrl := os.Getenv("OPENROUTER_BASE_URL")

	if local == "true" {
		model = "z-ai/glm-4.5-air:free"
	} else {
		model = "anthropic/claude-haiku-4.5"
	}

	if baseUrl == "" {
		baseUrl = "https://openrouter.ai/api/v1"
	}

	if apiKey == "" {
		panic("Env variable OPENROUTER_API_KEY not found")
	}

	client := openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseUrl))
	ctx := context.Background()

	params := openai.ChatCompletionNewParams{
		Model: model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(prompt),
					},
				},
			},
		},
		Tools: []openai.ChatCompletionToolUnionParam{
			openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
				Name:        "Read",
				Description: openai.String("Read and return the contents of a file"),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_path": map[string]string{
							"type":        "string",
							"description": "The path to the file to read",
						},
					},
					"required": []string{"file_path"},
				},
			}),
			openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
				Name:        "Write",
				Description: openai.String("Write content to a file"),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_path": map[string]string{
							"type":        "string",
							"description": "The path to the file to read",
						},
						"content": map[string]string{
							"type":        "string",
							"description": "The content to write to the file",
						},
					},
					"required": []string{"file_path", "content"},
				},
			}),
			openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
				Name:        "Bash",
				Description: openai.String("Execute a shell command"),
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]string{
							"type":        "string",
							"description": "The command to execute",
						},
					},
					"required": []string{"command"},
				},
			}),
		},
	}

	for {
		completion, err := client.Chat.Completions.New(ctx, params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(completion.Choices) == 0 {
			panic("No choices in response")
		}

		// You can use print statements as follows for debugging, they'll be visible when running tests.
		// fmt.Fprintln(os.Stderr, "Logs from your program will appear here!")

		toolCalls := completion.Choices[0].Message.ToolCalls

		params.Messages = append(params.Messages, completion.Choices[0].Message.ToParam())
		for _, toolCall := range toolCalls {
			if toolCall.Function.Name == "Read" {
				var args map[string]interface{}

				err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)

				if err != nil {
					panic(err)
				}

				file_path := args["file_path"].(string)

				fileData := readFilePath(file_path)

				params.Messages = append(params.Messages, openai.ToolMessage(fileData, toolCall.ID))
			}

			if toolCall.Function.Name == "Write" {
				var args map[string]interface{}

				err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)

				if err != nil {
					panic(err)
				}

				file_path := args["file_path"].(string)
				content := args["content"].(string)

				newContent := writeToFilePath(file_path, content)

				params.Messages = append(params.Messages, openai.ToolMessage(newContent, toolCall.ID))
			}

			if toolCall.Function.Name == "Bash" {
				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)

				if err != nil {
					panic(err)
				}
				cmd := args["command"].(string)
				data := execBash(cmd)

				params.Messages = append(params.Messages, openai.ToolMessage(data, toolCall.ID))
			}
		}

		if len(toolCalls) == 0 {
			fmt.Printf(completion.Choices[0].Message.Content)
			break
		}
	}

}

func readFilePath(path string) string {
	absPath, err := filepath.Abs(path)

	if err != nil {
		log.Printf("error reading the relative path")
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		log.Printf("file reading error: %v\n", err)
	}

	return string(content)
}

func writeToFilePath(path string, content string) string {
	var err error
	absPath, err := filepath.Abs(path)

	if err != nil {
		log.Printf("error reading the relative path")
	}

	file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)

	if err != nil {
		log.Printf("File opening error: %v\n", err)
	}

	defer file.Close()

	_, err = file.WriteString(content)

	if err != nil {
		log.Printf("File writing error %v\n", err)
	}

	return readFilePath(path)

}

func execBash(arguments string) string {
	cmd := exec.Command("sh", "-c", arguments)

	out, err := cmd.CombinedOutput()

	if err != nil {
		return err.Error()
	}

	return string(out)
}

