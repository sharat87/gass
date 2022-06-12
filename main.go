package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sharat87/gass/github"
	"github.com/sharat87/gass/parseargs"
	"golang.org/x/crypto/nacl/box"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

var ( // Injected at biuld time.
	Version string
	Commit  string
	Date    string
)

// Styles codes from <https://stackoverflow.com/a/33206814/151048>.
const STYLE_RESET = "\033[0m"
const STYLE_RED = "\033[31m"
const STYLE_GREEN = "\033[32m"
const STYLE_BLUE = "\033[34m"
const STYLE_MAGENTA = "\033[33m"
const STYLE_BOLD = "\033[1m"
const STYLE_REVERSE = "\033[7m"

type GithubResponseError struct {
	Message          string
	DocumentationUrl string `json:"documentation_url"`
}

type PublicKey struct {
	KeyId string `json:"key_id"`
	Key   string `json:"key"`
}

type SecretValue struct {
	Type    string
	Value   string
	FromEnv string `yaml:"from_env"`
}

type SecretValueSpec struct {
	Value            string
	FromEnv          string
	OrgVisibility    string   `yaml:"visibility"`
	OrgSelectedRepos []string `yaml:"selected_repos"`
}

type SecretPack struct {
	Secrets map[string]SecretValueSpec
}

type SyncSpecRepo struct {
	Delete  bool `yaml:"delete_unspecified"`
	Secrets map[string]SecretValueSpec
	Envs    map[string]SecretPack
}

type SyncSpecOrg struct {
	Delete  bool `yaml:"delete_unspecified"`
	Secrets map[string]SecretValueSpec
}

type SyncSpec struct {
	Vars  interface{}
	Repos map[string]SyncSpecRepo
	Orgs  map[string]SyncSpecOrg
}

type QualifiedSecretCallsByRepo struct {
	KeyId        string
	FullRepoName string
	Calls        []QualifiedSecretCall
	UsedSecrets  map[string]map[string]interface{}
	Envs         map[string]QualifiedSecretCallsByRepoEnv
}

type QualifiedSecretCallsByRepoEnv struct {
	Calls       []QualifiedSecretCall
	UsedSecrets map[string]map[string]interface{}
}

type QualifiedSecretCallsByOrg struct {
	KeyId   string
	OrgName string
	Calls   []QualifiedSecretCall
	// TODO: We aren't inspecting used secrets in orgs yet.
	UsedSecrets map[string]map[string]interface{}
}

type QualifiedSecretCall struct {
	Call           string // "create", "update", or "delete".
	SecretName     string
	EncryptedValue string // empty if `Call` is "delete".
	OrgVisibility  string // "org", "private", or "selected".
	OrgRepoIds     []int  // only applicable if `OrgVisibility` is "selected".
}

func (sv SecretValue) GetRealizedValue() (string, error) {
	if sv.Type == "value" {
		return sv.Value, nil
	} else if sv.Type == "fromEnv" {
		return os.Getenv(sv.FromEnv), nil
	}
	return "", errors.New("Invalue type in SecretValue " + sv.Type)
}

func (sv SecretValueSpec) GetRealizedValue() (string, error) {
	if sv.FromEnv == "" {
		return sv.Value, nil
	} else if sv.Value == "" {
		return os.Getenv(sv.FromEnv), nil
	}
	return "", errors.New("Both `Value` and `FromEnv` were provided in SecretValueSpec")
}

func main() {
	ia := parseargs.ParseArgs(os.Args[1:])

	fmt.Printf("gass version:%v commit:%v built:%v\n", Version, Commit, Date)

	if ia.Files == nil {
		log.Println("Please specify at least one `--file`.")
		return
	}

	if ia.IsDry {
		fmt.Print("\n" + STYLE_RED + "***    Dry run    ***" + STYLE_RESET + "\n\n")
	}

	allChanges := []QualifiedSecretCallsByRepo{}
	allChangesForOrgs := []QualifiedSecretCallsByOrg{}

	haveErrors := false

	for _, file := range ia.Files {
		secretsConfig := loadYaml(file)

		for repoName, repo := range secretsConfig.Repos {
			publicKey, err := getPublicKey(repoName)
			if err != nil {
				haveErrors = true
				log.Printf("Error getting public-key for repo '%v', due to '%v'", repoName, err)
				continue
			}
			thisRepoChanges := computeCalls(repoName, repo, publicKey, ia.IsDry)
			thisRepoChanges.KeyId = publicKey.KeyId
			thisRepoChanges.UsedSecrets, _ = getUsedSecrets(repoName)
			allChanges = append(allChanges, *thisRepoChanges)
		}

		for name, org := range secretsConfig.Orgs {
			publicKey, err := getPublicKeyForOrg(name)
			if err != nil {
				haveErrors = true
				log.Printf("Error getting public-key for org '%v', due to '%v'", name, err)
				continue
			}
			thisOrgChanges := computeCallsForOrg(name, org, publicKey, ia.IsDry)
			thisOrgChanges.KeyId = publicKey.KeyId
			// thisOrgChanges.UsedSecrets, _ = getUsedSecrets(org.Name)
			allChangesForOrgs = append(allChangesForOrgs, *thisOrgChanges)
		}
	}

	if haveErrors {
		log.Fatalln("Errors detected. Not doing anything. Please rectify and retry.")
	}

	// Also find used secrets that aren't set on the repo, and aren't given in the yml file here either.
	isUsedSecretsSetForDeletion := 0

	for _, org := range allChangesForOrgs {
		fmt.Println(STYLE_BOLD + "org  " + org.OrgName + STYLE_RESET)

		specifiedSecrets := map[string]interface{}{}

		for _, call := range org.Calls {
			if call.Call == "delete" {
				msg := "\t" + STYLE_RED + "deleted\t" + call.SecretName

				if _, ok := org.UsedSecrets[call.SecretName]; ok {
					isUsedSecretsSetForDeletion += 1
					msg += STYLE_BOLD + " " + STYLE_REVERSE + "(used in "
					isFirst := true
					for file, _ := range org.UsedSecrets[call.SecretName] {
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

		for usedSecret, _ := range org.UsedSecrets {
			if _, ok := specifiedSecrets[usedSecret]; !ok {
				fmt.Println("\t" + STYLE_MAGENTA + "missing\t" + usedSecret + STYLE_RESET)
			}
		}

		fmt.Println("")
	}

	for _, repo := range allChanges {
		fmt.Println(STYLE_BOLD + "repo " + repo.FullRepoName + STYLE_RESET)

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

		for envName, envChanges := range repo.Envs {
			fmt.Println("\t" + STYLE_BOLD + "env " + envName + STYLE_RESET)
			for _, call := range envChanges.Calls {
				if call.Call == "delete" {
					msg := "\t\t" + STYLE_RED + "deleted\t" + call.SecretName

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
					fmt.Println("\t\t" + STYLE_GREEN + "created\t" + call.SecretName + STYLE_RESET)
					specifiedSecrets[call.SecretName] = nil

				} else if call.Call == "update" {
					// TODO: Check if this is an unused secret, and if yes, show a info message.
					fmt.Println("\t\t" + STYLE_BLUE + "updated\t" + call.SecretName + STYLE_RESET)
					specifiedSecrets[call.SecretName] = nil

				}
			}
		}

		fmt.Println("")
	}

	if isUsedSecretsSetForDeletion > 0 {
		fmt.Println(
			STYLE_RED + "Some secrets that are used in workflows are set for deletion. Exiting without doing anything. Please review above output, resolve this and run again." + STYLE_RESET,
		)
	}

	// TODO: Before applying anything, ensure all required things exist, like repos, orgs, envs etc.
	if ia.IsDry {
		fmt.Println(STYLE_RED + "Not applying anything, since this is a dry run." + STYLE_RESET)
	} else {
		applyChanges(allChanges, allChangesForOrgs)
	}
}

func applyChanges(allChanges []QualifiedSecretCallsByRepo, allChangesForOrgs []QualifiedSecretCallsByOrg) {
	for _, orgChanges := range allChangesForOrgs {
		for _, call := range orgChanges.Calls {
			if call.Call == "delete" {
				err := github.DeleteSecretForOrg(orgChanges.OrgName, call.SecretName)
				if err != nil {
					log.Printf("Error deleting secret on GitHub %v/%v: %v", orgChanges.OrgName, call.SecretName, err)
					continue
				}

			} else if call.Call == "create" || call.Call == "update" {
				log.Printf("repo ids %v", call.OrgRepoIds)
				err := github.PutSecretForOrg(orgChanges.OrgName, call.SecretName, orgChanges.KeyId, call.EncryptedValue, call.OrgVisibility, call.OrgRepoIds)
				if err != nil {
					log.Printf("Error putting secret to GitHub %v/%v: %v", orgChanges.OrgName, call.SecretName, err)
					continue
				}

			}
		}
	}

	for _, repoChanges := range allChanges {
		for _, call := range repoChanges.Calls {
			if call.Call == "delete" {
				err := github.DeleteSecret(repoChanges.FullRepoName, call.SecretName)
				if err != nil {
					log.Printf("Error deleting secret on GitHub %v/%v: %v", repoChanges.FullRepoName, call.SecretName, err)
					continue
				}

			} else if call.Call == "create" || call.Call == "update" {
				err := github.PutSecret(repoChanges.FullRepoName, call.SecretName, repoChanges.KeyId, call.EncryptedValue)
				if err != nil {
					log.Printf("Error putting secret to GitHub %v/%v: %v", repoChanges.FullRepoName, call.SecretName, err)
					continue
				}

			}
		}

		for envName, envChanges := range repoChanges.Envs {
			for _, call := range envChanges.Calls {
				if call.Call == "delete" {
					err := github.DeleteSecretForEnv(repoChanges.FullRepoName, envName, call.SecretName)
					if err != nil {
						log.Printf("Error deleting env secret on GitHub %v/%v/%v: %v", repoChanges.FullRepoName, envName, call.SecretName, err)
						continue
					}

				} else if call.Call == "create" || call.Call == "update" {
					err := github.PutSecretForEnv(repoChanges.FullRepoName, envName, call.SecretName, repoChanges.KeyId, call.EncryptedValue)
					if err != nil {
						log.Printf("Error putting env secret to GitHub %v/%v/%v: %v", repoChanges.FullRepoName, envName, call.SecretName, err)
						continue
					}

				}
			}
		}
	}
}

func computeCalls(fullRepoName string, spec SyncSpecRepo, publicKey PublicKey, isDry bool) *QualifiedSecretCallsByRepo {
	changes := &QualifiedSecretCallsByRepo{
		KeyId:        publicKey.KeyId,
		FullRepoName: fullRepoName,
		Calls:        []QualifiedSecretCall{},
	}

	if spec.Envs != nil {
		changes.Envs = map[string]QualifiedSecretCallsByRepoEnv{}
	}

	existingSecretNames := map[string]interface{}{}

	for _, name := range getSecretList(fullRepoName) {
		existingSecretNames[name] = nil
	}

	for name, valueSpec := range spec.Secrets {
		stringValue, err := valueSpec.GetRealizedValue()
		if err != nil {
			log.Printf("Error getting realized value %v/%v: %v", fullRepoName, name, err)
			continue
		}

		encryptedValue, err := encrypt(publicKey.Key, stringValue)
		if err != nil {
			log.Printf("Error encrypting value for secret %v/%v: %v", fullRepoName, name, err)
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

	for envName, secretPack := range spec.Envs {
		envChanges := QualifiedSecretCallsByRepoEnv{
			Calls: []QualifiedSecretCall{},
		}

		existingSecretNamesForEnv := map[string]interface{}{}

		for _, name := range getSecretListForEnv(fullRepoName, envName) {
			existingSecretNamesForEnv[name] = nil
		}

		for name, valueSpec := range secretPack.Secrets {
			stringValue, err := valueSpec.GetRealizedValue()
			if err != nil {
				log.Printf("Error getting realized value %v/%v: %v", fullRepoName, name, err)
				continue
			}

			encryptedValue, err := encrypt(publicKey.Key, stringValue)
			if err != nil {
				log.Printf("Error encrypting value for secret %v/%v: %v", fullRepoName, name, err)
				continue
			}

			// TODO: Get secret list for this env here.
			call := "create"
			if _, ok := existingSecretNamesForEnv[name]; ok {
				call = "update"
			}

			envChanges.Calls = append(envChanges.Calls, QualifiedSecretCall{
				Call:           call,
				SecretName:     name,
				EncryptedValue: encryptedValue,
			})

			if spec.Delete {
				delete(existingSecretNamesForEnv, name)
			}
		}

		if spec.Delete {
			for name, _ := range existingSecretNamesForEnv {
				envChanges.Calls = append(envChanges.Calls, QualifiedSecretCall{
					Call:       "delete",
					SecretName: name,
				})
			}
		}

		changes.Envs[envName] = envChanges
	}

	return changes
}

func computeCallsForOrg(orgName string, spec SyncSpecOrg, publicKey PublicKey, isDry bool) *QualifiedSecretCallsByOrg {
	changes := &QualifiedSecretCallsByOrg{
		KeyId:   publicKey.KeyId,
		OrgName: orgName,
		Calls:   []QualifiedSecretCall{},
	}

	existingSecretNames := map[string]interface{}{}

	for _, name := range getSecretListForOrg(orgName) {
		existingSecretNames[name] = nil
	}

	repoIds := getRepoIdsForOrg(orgName)

	for name, valueSpec := range spec.Secrets {
		stringValue, err := valueSpec.GetRealizedValue()
		if err != nil {
			log.Printf("Error getting realized value %v/%v: %v", orgName, name, err)
			continue
		}

		encryptedValue, err := encrypt(publicKey.Key, stringValue)
		if err != nil {
			log.Printf("Error encrypting value for secret %v/%v: %v", orgName, name, err)
			continue
		}

		var thisRepoIds []int
		if valueSpec.OrgVisibility == "selected" {
			thisRepoIds = []int{}
			for _, selectedRepoName := range valueSpec.OrgSelectedRepos {
				thisRepoIds = append(thisRepoIds, repoIds[selectedRepoName])
			}
		}

		call := "create"
		if _, ok := existingSecretNames[name]; ok {
			call = "update"
		}

		changes.Calls = append(changes.Calls, QualifiedSecretCall{
			Call:           call,
			SecretName:     name,
			EncryptedValue: encryptedValue,
			OrgVisibility:  valueSpec.OrgVisibility,
			OrgRepoIds:     thisRepoIds,
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

func getRepoIdsForOrg(name string) map[string]int {
	body, err := github.MakeGitHubRequest("GET", "orgs/"+name+"/repos?per_page=100", nil)
	if err != nil {
		log.Fatalln(err)
	}

	type Repo struct {
		Id   int
		Name string
	}

	repos := []Repo{}
	err = json.Unmarshal(body, &repos)
	if err != nil {
		log.Fatalln(err)
	}

	repoIdsByName := map[string]int{}
	for _, repo := range repos {
		repoIdsByName[repo.Name] = repo.Id
	}

	return repoIdsByName
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

func getSecretList(fullRepoName string) []string {
	body, err := github.MakeGitHubRequest("GET", "repos/"+fullRepoName+"/actions/secrets", nil)
	if err != nil {
		log.Fatalln(err)
	}

	type Secret struct {
		Name      string
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	type Response struct {
		TotalCount int `json:"total_count"`
		Secrets    []Secret
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

func getSecretListForEnv(fullRepoName string, envName string) []string {
	body, err := github.MakeGitHubRequest("GET", "/repos/"+fullRepoName, nil)
	if err != nil {
		log.Fatalln(err)
	}

	type RepoResponse struct {
		Id string
	}

	var repoResponse RepoResponse
	err = json.Unmarshal(body, &repoResponse)
	if err != nil {
		log.Fatalln(err)
	}

	body, err = github.MakeGitHubRequest("GET", "/repositories/"+repoResponse.Id+"/environments/"+envName+"/secrets", nil)
	if err != nil {
		log.Fatalln(err)
	}

	type Secret struct {
		Name      string
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	type Response struct {
		TotalCount int `json:"total_count"`
		Secrets    []Secret
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

func getSecretListForOrg(name string) []string {
	body, err := github.MakeGitHubRequest("GET", "orgs/"+name+"/actions/secrets", nil)
	if err != nil {
		log.Fatalln(err)
	}

	type Secret struct {
		Name                string
		CreatedAt           string `json:"created_at"`
		UpdatedAt           string `json:"updated_at"`
		OrgVisibility       string `json:"visibility"`
		OrgSelectedReposUrl string `json:"selected_repositories_url"`
	}

	type Response struct {
		TotalCount int `json:"total_count"`
		Secrets    []Secret
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

func getPublicKey(fullRepoName string) (PublicKey, error) {
	response := PublicKey{}

	body, err := github.MakeGitHubRequest("GET", "repos/"+fullRepoName+"/actions/secrets/public-key", nil)
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

func getPublicKeyForOrg(name string) (PublicKey, error) {
	response := PublicKey{}

	body, err := github.MakeGitHubRequest("GET", "orgs/"+name+"/actions/secrets/public-key", nil)
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

func getUsedSecrets(fullRepoName string) (map[string]map[string]interface{}, error) {
	type Item struct {
		Name        string
		DownloadURL string `json:"download_url"`
	}

	filesBySecret := map[string]map[string]interface{}{}

	body, _ := github.MakeGitHubRequest("GET", "repos/"+fullRepoName+"/contents/.github/workflows", nil)

	items := []Item{}
	err := json.Unmarshal(body, &items)
	if err != nil {
		errResponse := GithubResponseError{}
		err := json.Unmarshal(body, &errResponse)
		if err != nil {
			log.Printf("Error unmarshalling get-workflows error response: %v", err)
		}
		if errResponse.Message == "This repository is empty." {
			return filesBySecret, nil
		}
		if errResponse.Message != "" {
			log.Fatalf("Error getting public-key for %v: %v", fullRepoName, errResponse.Message)
		}

		log.Printf("Response from workflows: %v", string(body))
		log.Fatalf("Getting workflows: %v", err)
	}

	secretsPat := regexp.MustCompile(`\${{\s*secrets\.([^}\s]+)\s*}}`)

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
