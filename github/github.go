package github

import (
	"fmt"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"strconv"
)

var GITHUB_TOKEN = os.Getenv("GITHUB_API_TOKEN")

type PublicKey struct {
	KeyId string `json:"key_id"`
	Key   string `json:"key"`
}

type GithubResponseError struct {
	Message          string
	DocumentationUrl string `json:"documentation_url"`
}

func MakeGitHubRequest(method, path string, body interface{}) ([]byte, error) {
	var requestBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		requestBody = bytes.NewBuffer(data)
	}

	req, err := http.NewRequest(method, "https://api.github.com/"+path, requestBody)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", "application/vnd.github.v3+json")
	req.Header.Add("Authorization", "Bearer "+GITHUB_TOKEN)

	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return responseBody, nil
}

func getRepoId(fullRepoName string) string {
	body, err := MakeGitHubRequest("GET", "repos/"+fullRepoName, nil)
	if err != nil {
		log.Fatalln(err)
	}

	type Repo struct {
		Id   int
	}

	repo := Repo{}
	err = json.Unmarshal(body, &repo)
	if err != nil {
		log.Fatalln(err)
	}

	return strconv.Itoa(repo.Id)
}

func PutSecret(fullRepoName, secretName, keyId, encryptedValueStr string) error {
	body := map[string]string{
		"encrypted_value": encryptedValueStr,
		"key_id":          keyId,
	}

	_, err := MakeGitHubRequest("PUT", "repos/"+fullRepoName+"/actions/secrets/"+secretName, body)
	if err != nil {
		return err
	}

	return nil
}

func PutSecretForEnv(fullRepoName, envName, secretName, keyId, encryptedValueStr string) error {
	body := map[string]string{
		"encrypted_value": encryptedValueStr,
		"key_id":          keyId,
	}

	responseBody, err := MakeGitHubRequest("PUT", "/repositories/"+getRepoId(fullRepoName)+"/environments/"+envName+"/secrets/"+secretName, body)
	if err != nil {
		return err
	}
	log.Printf("Response from put secret for env %v/%v/%v: %v", fullRepoName, envName, secretName, string(responseBody))

	return nil
}

func PutSecretForOrg(name, secretName, keyId, encryptedValueStr, visibility string, selected_repository_ids []int) error {
	body := map[string]interface{}{
		"encrypted_value": encryptedValueStr,
		"key_id":          keyId,
		"visibility":      visibility,
	}

	if selected_repository_ids != nil {
		body["selected_repository_ids"] = selected_repository_ids
	}

	responseBody, err := MakeGitHubRequest("PUT", "orgs/"+name+"/actions/secrets/"+secretName, body)
	if err != nil {
		return err
	}
	// TODO: Handle potential error messages, when the response is `{"message":"Invalid request.\n\nFor 'items', \"491327810\" is not an integer.","documentation_url":"https://docs.github.com/rest/reference/actions#create-or-update-an-organization-secret"}`
	log.Printf("Response from saving org secret %v", string(responseBody))

	return nil
}

func DeleteSecret(fullRepoName, secretName string) error {
	_, err := MakeGitHubRequest("DELETE", "repos/"+fullRepoName+"/actions/secrets/"+secretName, nil)
	if err != nil {
		return err
	}

	return nil
}

func DeleteSecretForEnv(fullRepoName, envName, secretName string) error {
	_, err := MakeGitHubRequest("DELETE", "/repositories/"+getRepoId(fullRepoName)+"/environments/"+envName+"/secrets/"+secretName, nil)
	if err != nil {
		return err
	}

	return nil
}

func DeleteSecretForOrg(name, secretName string) error {
	_, err := MakeGitHubRequest("DELETE", "orgs/"+name+"/actions/secrets/"+secretName, nil)
	if err != nil {
		return err
	}

	return nil
}

func FetchPublicKey(fullRepoName string) (PublicKey, error) {
	response := PublicKey{}

	body, err := MakeGitHubRequest("GET", "repos/"+fullRepoName+"/actions/secrets/public-key", nil)
	if err != nil {
		return response, err
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return response, fmt.Errorf("Error unmarshalling public-key response: %v", err)
	}

	if response.Key == "" {
		errResponse := GithubResponseError{}
		err = json.Unmarshal(body, &errResponse)
		if err != nil {
			return response, fmt.Errorf("Error unmarshalling public-key error response: %v", err)
		}
		if errResponse.Message != "" {
			return response, fmt.Errorf(errResponse.Message)
		}
	}

	return response, nil
}

func FetchPublicKeyForOrg(name string) (PublicKey, error) {
	response := PublicKey{}

	body, err := MakeGitHubRequest("GET", "orgs/"+name+"/actions/secrets/public-key", nil)
	if err != nil {
		return response, err
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return response, fmt.Errorf("Error unmarshalling public-key for org response: %v", err)
	}

	if response.Key == "" {
		errResponse := GithubResponseError{}
		err = json.Unmarshal(body, &errResponse)
		if err != nil {
			return response, fmt.Errorf("Error unmarshalling public-key error response: %v", err)
		}
		if errResponse.Message != "" {
			return response, fmt.Errorf(errResponse.Message)
		}
	}

	return response, nil
}

func FetchUsedSecrets(fullRepoName string) (map[string]map[string]interface{}, error) {
	workflows, err := downloadWorkflows(fullRepoName)
	if err != nil {
		return nil, err
	}

	return CollectFilesBySecret(workflows), nil
}

func downloadWorkflows(repo string) (map[string][]byte, error) {
	type Item struct {
		Name        string
		DownloadURL string `json:"download_url"`
	}

	workflows := map[string][]byte{}

	body, _ := MakeGitHubRequest("GET", "repos/"+repo+"/contents/.github/workflows", nil)

	items := []Item{}
	err := json.Unmarshal(body, &items)
	if err != nil {
		errResponse := GithubResponseError{}
		err := json.Unmarshal(body, &errResponse)
		if err != nil {
			return nil, err
		}

		if errResponse.Message == "This repository is empty." {
			// To us, an empty repository is not an error.
			return workflows, nil
		}

		if errResponse.Message == "" {
			return nil, fmt.Errorf("Empty error message getting workflow file list")
		}

		return nil, fmt.Errorf(errResponse.Message)
	}

	for _, item := range items {
		if !strings.HasSuffix(item.Name, ".yml") && !strings.HasSuffix(item.Name, ".yaml") {
			continue
		}

		resp, err := http.Get(item.DownloadURL)
		if err != nil {
			return nil, err
		}

		responseBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		workflows[item.Name] = responseBody
	}

	return workflows, nil
}

func CollectFilesBySecret(workflows map[string][]byte) map[string]map[string]interface{} {
	filesBySecret := map[string]map[string]interface{}{}
	secretsPat := regexp.MustCompile(`\${{\s*secrets\.([^}\s]+)\s*}}`)

	for filename, content := range workflows {
		for _, match := range secretsPat.FindAllSubmatch(content, -1) {
			secretName := string(match[1])
			if _, ok := filesBySecret[secretName]; !ok {
				filesBySecret[secretName] = map[string]interface{}{}
			}
			filesBySecret[secretName][filename] = nil
		}
	}

	return filesBySecret
}
