package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/nacl/box"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

var GITHUB_TOKEN = os.Getenv("GITHUB_API_TOKEN")

type PublicKey struct {
	KeyId string `json:"key_id"`
	Key   string `json:"key"`
}

type SyncSpecRepo struct {
	Owner   string            `yaml:"owner"`
	Name    string            `yaml:"name"`
	Delete  bool              `yaml:"deleteUnspecified"`
	Secrets map[string]string `yaml:"secrets"`
}

type SyncSpec struct {
	Repos []SyncSpecRepo `yaml:"repos"`
}

func main() {
	secretsConfig := loadYaml("secrets.yml")

	for _, repo := range secretsConfig.Repos {
		log.Printf("Syncing repository %s/%s", repo.Owner, repo.Name)
		applySyncSpec(repo)
	}

	log.Println("Done")
}

func applySyncSpec(spec SyncSpecRepo) {
	existingSecretNames := map[string]interface{}{}

	if spec.Delete {
		for _, name := range getSecretList(spec.Owner, spec.Name) {
			existingSecretNames[name] = nil
		}
	}

	publicKey := getPublicKey(spec.Owner, spec.Name)
	for name, value := range spec.Secrets {
		encryptedValue, err := encrypt(publicKey.Key, value)
		if err != nil {
			log.Printf("Error encrypting value for secret %v/%v/%v: %v", spec.Owner, spec.Name, name, err)
			continue
		}
		err = putSecret(spec.Owner, spec.Name, name, publicKey.KeyId, encryptedValue)
		if err != nil {
			log.Printf("Error putting secret to GitHub %v/%v/%v: %v", spec.Owner, spec.Name, name, err)
			continue
		}
		if spec.Delete {
			delete(existingSecretNames, name)
		}
	}

	if spec.Delete {
		for name, _ := range existingSecretNames {
			err := deleteSecret(spec.Owner, spec.Name, name)
			if err != nil {
				log.Printf("Error deleting secret on GitHub %v/%v/%v: %v", spec.Owner, spec.Name, name, err)
				continue
			}
		}
	}
}

func encrypt(key, value string) (string, error) {
	decodedKey, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", err
	}

	// Ref <https://stackoverflow.com/a/67199587/151048> for syntax used in third argument.
	encryptedValue, err := box.SealAnonymous([]byte{}, []byte(value), (*[32]byte)(decodedKey), rand.Reader)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(encryptedValue), nil
}

func getSecretList(owner, repo string) []string {
	body, err := makeGithubRequest("GET", "repos/"+owner+"/"+repo+"/actions/secrets", nil)
	if err != nil {
		log.Fatalln(err)
	}

	type Secret struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	type Response struct {
		TotalCount int      `json:"total_count"`
		Secrets    []Secret `json:"secrets"`
	}

	var response Response
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Fatalln(err)
	}

	names := []string{}
	for _, secret := range response.Secrets {
		names = append(names, secret.Name)
	}

	return names
}

func getPublicKey(owner, repo string) PublicKey {
	body, err := makeGithubRequest("GET", "repos/"+owner+"/"+repo+"/actions/secrets/public-key", nil)
	if err != nil {
		log.Fatalln(err)
	}

	response := PublicKey{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		log.Fatalln(err)
	}

	return response
}

func putSecret(owner, repo, secretName, keyId, encryptedValueStr string) error {
	log.Printf("Putting secret %v\n", secretName)

	body := map[string]string{
		"encrypted_value": encryptedValueStr,
		"key_id":          keyId,
	}

	_, err := makeGithubRequest("PUT", "repos/"+owner+"/"+repo+"/actions/secrets/"+secretName, body)
	if err != nil {
		return err
	}

	return nil
}

func deleteSecret(owner, repo, secretName string) error {
	log.Printf("Deleting secret %v\n", secretName)

	_, err := makeGithubRequest(
		"DELETE",
		"repos/"+owner+"/"+repo+"/actions/secrets/"+secretName,
		nil,
	)
	if err != nil {
		return err
	}

	return nil
}

func makeGithubRequest(method, path string, body interface{}) ([]byte, error) {
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

func loadYaml(filename string) SyncSpec {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println(err)
	}
	defer file.Close()

	byteValue, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Println(err)
	}

	data := SyncSpec{}
	yaml.UnmarshalStrict(byteValue, &data)

	return data
}
