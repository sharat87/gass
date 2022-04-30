package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/nacl/box"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

var GITHUB_TOKEN = os.Getenv("GITHUB_API_TOKEN")

// Styles codes from <https://stackoverflow.com/a/33206814/151048>.
const STYLE_RESET = "\033[0m"
const STYLE_RED = "\033[31m"
const STYLE_GREEN = "\033[32m"
const STYLE_BLUE = "\033[34m"
const STYLE_MAGENTA = "\033[33m"
const STYLE_BOLD = "\033[1m"
const STYLE_REVERSE = "\033[7m"

type InvokeArgs struct {
	Action string
	IsDry  bool
	Files  []string
}

type PublicKey struct {
	KeyId string `json:"key_id"`
	Key   string `json:"key"`
}

type SecretValue struct {
	Type    string
	Value   string
	FromEnv string
}

type SyncSpecRepo struct {
	Owner   string
	Name    string
	Delete  bool `yaml:"deleteUnspecified"`
	Secrets map[string]SecretValue
}

type SyncSpec struct {
	Vars  interface{}
	Repos []SyncSpecRepo
}

type QualifiedSecretCallsByRepo struct {
	KeyId       string
	RepoOwner   string
	RepoName    string
	Calls       []QualifiedSecretCall
	UsedSecrets map[string]map[string]interface{}
}

type QualifiedSecretCall struct {
	Call           string // "create", "update", or "delete".
	SecretName     string
	EncryptedValue string // empty if `Call` is "delete".
}

func (sv *SecretValue) UnmarshalYAML(unmarshal func(interface{}) error) error {
	stringValue := ""
	if unmarshal(&stringValue) == nil {
		sv.Type = "value"
		sv.Value = stringValue
		return nil
	}

	mapValue := make(map[string]string)
	if unmarshal(&mapValue) == nil {
		if envName, ok := mapValue["fromEnv"]; ok {
			sv.Type = "fromEnv"
			sv.FromEnv = envName
			return nil
		}

		return errors.New("No `fromEnv` found in map value for secret.")
	}

	return errors.New("Invalid type for secret value")
}

func (sv SecretValue) GetRealizedValue() (string, error) {
	if sv.Type == "value" {
		return sv.Value, nil
	} else if sv.Type == "fromEnv" {
		return os.Getenv(sv.FromEnv), nil
	}
	return "", errors.New("Invalue type in SecretValue " + sv.Type)
}

func main() {
	ia := parseArgs(os.Args[1:])

	if ia.Files == nil {
		log.Println("Please specify at least one `--file`.")
		return
	}

	if ia.IsDry {
		fmt.Println("\n" + STYLE_RED + "***    Dry run    ***" + STYLE_RESET + "\n")
	}

	allChanges := []QualifiedSecretCallsByRepo{}

	for _, file := range ia.Files {

		secretsConfig := loadYaml(file)

		for _, repo := range secretsConfig.Repos {
			publicKey := getPublicKey(repo.Owner, repo.Name)
			thisRepoChanges := computeCalls(repo, publicKey, ia.IsDry)
			thisRepoChanges.KeyId = publicKey.KeyId
			thisRepoChanges.UsedSecrets, _ = getUsedSecrets(repo.Owner, repo.Name)
			allChanges = append(allChanges, *thisRepoChanges)
		}
	}

	// Also find used secrets that aren't set on the repo, and aren't given in the yml file here either.
	isUsedSecretsSetForDeletion := 0

	for _, repo := range allChanges {
		fmt.Println(STYLE_BOLD + repo.RepoOwner + "/" + repo.RepoName + STYLE_RESET)

		specifiedSecrets := map[string]interface{}{}

		for _, call := range repo.Calls {
			if call.Call == "delete" {
				msg := "\t" + STYLE_RED + "deleted\t" + call.SecretName

				if _, ok := repo.UsedSecrets[call.SecretName]; ok {
					isUsedSecretsSetForDeletion += 1
					msg += STYLE_BOLD + " " + STYLE_REVERSE + "(used in "
					isFirst := true
					for file, _ := range repo.UsedSecrets[call.SecretName] {
						msg += "'" + file + "'"
						if !isFirst {
							msg += ", "
						}
						isFirst = false
					}
					msg += ")"
				}

				fmt.Println(msg + STYLE_RESET)

			} else if call.Call == "create" {
				// TODO: Check if this is an unused secret, and if yes, show a info message.
				fmt.Println("\t" + STYLE_GREEN + "created\t" + call.SecretName + STYLE_RESET)
				specifiedSecrets[call.SecretName] = nil

			} else if call.Call == "update" {
				// TODO: Check if this is an unused secret, and if yes, show a info message.
				fmt.Println("\t" + STYLE_BLUE + "updated\t" + call.SecretName + STYLE_RESET)
				specifiedSecrets[call.SecretName] = nil

			}
		}

		for usedSecret, _ := range repo.UsedSecrets {
			if _, ok := specifiedSecrets[usedSecret]; !ok {
				fmt.Println("\t" + STYLE_MAGENTA + "missing\t" + usedSecret + STYLE_RESET)
			}
		}

		fmt.Println("")
	}

	if isUsedSecretsSetForDeletion > 0 {
		fmt.Println(
			STYLE_RED + "Some secrets that are used in workflows are set for deletion. Exiting without doing anything. Please review above output, resolve this and run again." + STYLE_RESET,
		)
	}

	if ia.IsDry {
		fmt.Println(STYLE_RED + "Not applying anything, since this is a dry run." + STYLE_RESET)
	} else {
		applyChanges(allChanges)
	}
}

func applyChanges(allChanges []QualifiedSecretCallsByRepo) {
	for _, repoChanges := range allChanges {
		for _, call := range repoChanges.Calls {
			if call.Call == "delete" {
				err := deleteSecret(repoChanges.RepoOwner, repoChanges.RepoName, call.SecretName)
				if err != nil {
					log.Printf("Error deleting secret on GitHub %v/%v/%v: %v", repoChanges.RepoOwner, repoChanges.RepoName, call.SecretName, err)
					continue
				}

			} else if call.Call == "create" || call.Call == "update" {
				err := putSecret(repoChanges.RepoOwner, repoChanges.RepoName, call.SecretName, repoChanges.KeyId, call.EncryptedValue)
				if err != nil {
					log.Printf("Error putting secret to GitHub %v/%v/%v: %v", repoChanges.RepoOwner, repoChanges.RepoName, call.SecretName, err)
					continue
				}

			}
		}
	}
}

func parseArgs(args []string) InvokeArgs {
	ia := &InvokeArgs{
		Action: "",
		IsDry:  false,
		Files:  nil,
	}

	state := ""

	for _, arg := range args {
		if state == "file" {
			state = ""
			if ia.Files == nil {
				ia.Files = []string{}
			}
			ia.Files = append(ia.Files, arg)

		} else if arg == "--dry" {
			ia.IsDry = true

		} else if arg == "--file" {
			state = "file"

		}
	}

	return *ia
}

func computeCalls(spec SyncSpecRepo, publicKey PublicKey, isDry bool) *QualifiedSecretCallsByRepo {
	changes := &QualifiedSecretCallsByRepo{
		KeyId:     publicKey.KeyId,
		RepoOwner: spec.Owner,
		RepoName:  spec.Name,
		Calls:     []QualifiedSecretCall{},
	}

	existingSecretNames := map[string]interface{}{}

	for _, name := range getSecretList(spec.Owner, spec.Name) {
		existingSecretNames[name] = nil
	}

	for name, value := range spec.Secrets {
		stringValue, err := value.GetRealizedValue()
		if err != nil {
			log.Printf("Error getting realized value %v/%v/%v: %v", spec.Owner, spec.Name, name, err)
			continue
		}

		encryptedValue, err := encrypt(publicKey.Key, stringValue)
		if err != nil {
			log.Printf("Error encrypting value for secret %v/%v/%v: %v", spec.Owner, spec.Name, name, err)
			continue
		}

		call := "create"
		if _, ok := existingSecretNames[name]; ok {
			call = "update"
		}

		changes.Calls = append(changes.Calls, QualifiedSecretCall{
			Call:           call,
			SecretName:     name,
			EncryptedValue: encryptedValue,
		})

		if spec.Delete {
			delete(existingSecretNames, name)
		}
	}

	if spec.Delete {
		for name, _ := range existingSecretNames {
			changes.Calls = append(changes.Calls, QualifiedSecretCall{
				Call:       "delete",
				SecretName: name,
			})
		}
	}

	return changes
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
	_, err := makeGithubRequest("DELETE", "repos/"+owner+"/"+repo+"/actions/secrets/"+secretName, nil)
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

func getUsedSecrets(owner, repo string) (map[string]map[string]interface{}, error) {
	type Item struct {
		Name        string
		DownloadURL string `json:"download_url"`
	}

	body, _ := makeGithubRequest("GET", "repos/"+owner+"/"+repo+"/contents/.github/workflows", nil)

	items := []Item{}
	err := json.Unmarshal(body, &items)
	if err != nil {
		log.Fatalln(err)
	}

	secretsPat := regexp.MustCompile(`\${{\s*secrets\.([^}\s]+)\s*}}`)

	filesBySecret := map[string]map[string]interface{}{}

	for _, item := range items {
		if !strings.HasSuffix(item.Name, ".yml") && !strings.HasPrefix(item.Name, ".yaml") {
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

		for _, match := range secretsPat.FindAllSubmatch(responseBody, -1) {
			secretName := string(match[1])
			if _, ok := filesBySecret[secretName]; !ok {
				filesBySecret[secretName] = map[string]interface{}{}
			}
			filesBySecret[secretName][item.Name] = nil
		}

		break
	}

	return filesBySecret, nil
}
