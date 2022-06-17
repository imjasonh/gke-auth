package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2/google"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

var (
	project     = flag.String("project", "", "Name of the project (empty means look it up)")
	location    = flag.String("location", "", "Location of the cluster (empty means look it up)")
	clusterName = flag.String("cluster", "", "Name of the cluster")

	get   = flag.Bool("get", false, "If true, print auth information")
	clear = flag.Bool("clear", false, "If true, clear auth for this cluster")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	if *get && *clear {
		log.Fatal("cannot pass both --get and --clear")
	}
	if !*get && (*project == "" || *location == "" || *clusterName == "") {
		log.Fatal("must pass --project and --location and --cluster")
	}

	// get a token
	ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		log.Fatalf("google.DefaultTokenSource: %v", err)
	}
	tok, err := ts.Token()
	if err != nil {
		log.Fatalf("ts.Token: %v", err)
	}

	if *get {
		// just print the token
		if err := json.NewEncoder(os.Stdout).Encode(&clientauthv1.ExecCredential{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "client.authentication.k8s.io/v1",
				Kind:       "ExecCredential",
			},
			Status: &clientauthv1.ExecCredentialStatus{
				ExpirationTimestamp: &metav1.Time{Time: tok.Expiry},
				Token:               tok.AccessToken,
			},
		}); err != nil {
			log.Fatalf("Encoding JSON: %v", err)
		}
		return
	}

	url := fmt.Sprintf("https://container.googleapis.com/v1/projects/%s/locations/%s/clusters/%s", *project, *location, *clusterName)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Fatalf("http.NewRequest: %v", err)
	}
	tok.SetAuthHeader(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("http.Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		all, _ := io.ReadAll(resp.Body)
		log.Fatalf("http.Do: %d %s %s", resp.StatusCode, resp.Status, string(all))
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

	cfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		log.Fatalf("Loading kubeconfig %v", err)
	}

	// get kubeconfig location
	key := fmt.Sprintf("gke_%s_%s_%s", *project, *location, *clusterName)
	kcfgPath := clientcmd.RecommendedHomeFile
	if auth := cfg.AuthInfos[key]; auth != nil && auth.LocationOfOrigin != "" {
		kcfgPath = auth.LocationOfOrigin
	}

	// add user
	if *clear {
		cfg.AuthInfos[key] = &api.AuthInfo{}
		cfg.Clusters[key] = &api.Cluster{}
		cfg.Contexts[key] = &api.Context{}
		cfg.CurrentContext = ""
		log.Println("Auth config cleared")
	} else {
		cfg.AuthInfos[key] = &api.AuthInfo{
			Exec: &api.ExecConfig{
				APIVersion:      "client.authentication.k8s.io/v1",
				Command:         os.Args[0],
				Args:            []string{"--get"},
				InteractiveMode: api.NeverExecInteractiveMode,
				InstallHint:     "go install github.com/imjasonh/gke-auto@latest",
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
	}

	// write the file back
	if err := clientcmd.WriteToFile(*cfg, kcfgPath); err != nil {
		log.Fatalf("Writing kubeconfig %q: %v", kcfgPath, err)
	}
}
