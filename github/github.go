package github

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
)

var GITHUB_TOKEN = os.Getenv("GITHUB_API_TOKEN")

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
