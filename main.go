package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mitchellh/go-homedir"
	"golang.org/x/oauth2/google"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

var (
	project     = flag.String("project", "", "Name of the project (empty means look it up)")
	location    = flag.String("location", "", "Location of the cluster (empty means look it up)")
	clusterName = flag.String("cluster", "", "Name of the cluster")

	get = flag.Bool("get", false, "If true, print auth information")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	// get a token
	ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/cloud-platform")
	if err != nil {
		log.Fatalf("google.DefaultTokenSource: %v", err)
	}
	tok, err := ts.Token()
	if err != nil {
		log.Fatalf("ts.Token: %v", err)
	}

	if *get {
		// just print the token
		if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
			"access_token": tok.AccessToken,
			"token_expiry": tok.Expiry.Format(time.RFC3339),
		}); err != nil {
			log.Fatalf("Encoding JSON: %v", err)
		}
		return
	}

	// get the GKE cluster
	url := fmt.Sprintf("https://container.googleapis.com/v1/projects/%s/locations/%s/clusters/%s", *project, *location, *clusterName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("http.Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		all, _ := ioutil.ReadAll(resp.Body)
		log.Fatal("GET %q: %d\n%s", url, resp.StatusCode, string(all))
	}
	var cluster struct {
		Endpoint   string
		MasterAuth struct {
			ClusterCaCertificate string
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&cluster); err != nil {
		log.Fatalf("json.Decode: %v", err)
	}

	// load the current kubeconfig
	kcfgPath := os.Getenv("KUBECONFIG")
	if kcfgPath == "" {
		kcfgPath, err = homedir.Expand("~/.kube/config")
		if err != nil {
			log.Fatalf("homedir: %v", err)
		}
	}
	dir := filepath.Dir(kcfgPath)
	if err := os.MkdirAll(dir, 0777); err != nil {
		log.Fatalf("mkdir -p %q: %v", dir, err)
	}
	if f, err := os.OpenFile(kcfgPath, os.O_CREATE|os.O_RDONLY, 0777); err != nil {
		log.Fatalf("open %q: %v", kcfgPath, err)
	} else {
		f.Close()
	}
	cfg, err := clientcmd.LoadFromFile(kcfgPath)
	if err != nil {
		log.Fatalf("Loading kubeconfig %q: %v", kcfgPath, err)
	}

	// add user
	key := fmt.Sprintf("gke_%s_%s_%s", *project, *location, *clusterName)
	cfg.AuthInfos[key] = &api.AuthInfo{
		AuthProvider: &api.AuthProviderConfig{
			Config: map[string]string{
				"access-token": tok.AccessToken,
				"cmd-args":     "--get",
				"cmd-path":     os.Args[0],
				"expiry":       tok.Expiry.Format(time.RFC3339),
				"expiry-key":   "{.token_expiry}",
				"token-key":    "{.access_token}",
			},
			Name: "gcp",
		},
	}

	// add cluster
	dec, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		log.Fatalf("Decoding CA cert: %v", err)
	}
	cfg.Clusters[key] = &api.Cluster{
		Server:                   "https://" + cluster.Endpoint,
		CertificateAuthorityData: dec,
	}

	// update current context
	cfg.Contexts[key] = &api.Context{
		AuthInfo: key,
		Cluster:  key,
	}
	cfg.CurrentContext = key

	// write the file back
	if err := clientcmd.WriteToFile(*cfg, kcfgPath); err != nil {
		log.Fatalf("Writing kubeconfig %q: %v", kcfgPath, err)
	}
}
