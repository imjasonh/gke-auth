package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/oauth2"
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

	get                = flag.Bool("get", false, "If true, print auth information")
	clear              = flag.Bool("clear", false, "If true, clear auth for this cluster")
	verbose            = flag.Bool("verbose", false, "If true, print debugging information about the plugin execution")
	skipPrivilegeCheck = flag.Bool("skip-privilege-check", false, "If true, skip checking for privilege status. Should only be used in non-interactive environments.")
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

	cfg, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		log.Fatalf("Loading kubeconfig %v", err)
	}

	// get kubeconfig location
	key := fmt.Sprintf("gke_%s_%s_%s", *project, *location, *clusterName)

	if *clear {
		cfg.AuthInfos[key] = &api.AuthInfo{}
		cfg.Clusters[key] = &api.Cluster{}
		cfg.Contexts[key] = &api.Context{}
		cfg.CurrentContext = ""
		log.Println("Auth config cleared")

		return
	}

	tok := getToken(ctx)
	vprintln("got oauth2 token")

	cluster := getCluster(ctx, tok)
	vprintln("got gke cluster")

	var privileged bool
	if !*skipPrivilegeCheck {
		privileged = checkClusterIsPrivileged(cluster)
		vprintln("checked for privileged cluster")
	} else {
		fmt.Fprintf(os.Stderr, "skipping privilege check for %v located in %v in project %v\n", *clusterName, *location, *project)
	}

	if *get {
		vprintln("getting token for cluster access")
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

	// add user
	authInfo := &api.AuthInfo{
		Exec: &api.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1",
			Command:    os.Args[0],
			Args: []string{
				"--get",
				fmt.Sprintf("--project=%v", *project),
				fmt.Sprintf("--location=%v", *location),
				fmt.Sprintf("--cluster=%v", *clusterName),
			},
			InteractiveMode: api.NeverExecInteractiveMode,
			InstallHint:     "go install github.com/imjasonh/gke-auto@latest",
		},
	}

	if privileged {
		authInfo.Exec.InteractiveMode = api.AlwaysExecInteractiveMode
	}

	if *verbose {
		authInfo.Exec.Args = append(authInfo.Exec.Args, "--verbose")
	}

	if *skipPrivilegeCheck {
		authInfo.Exec.Args = append(authInfo.Exec.Args, "--skip-privilege-check")
	}

	cfg.AuthInfos[key] = authInfo

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

	kcfgPath := clientcmd.RecommendedHomeFile
	if auth := cfg.AuthInfos[key]; auth != nil && auth.LocationOfOrigin != "" {
		kcfgPath = auth.LocationOfOrigin
	}

	// write the file back
	if err := clientcmd.WriteToFile(*cfg, kcfgPath); err != nil {
		log.Fatalf("Writing kubeconfig %q: %v", kcfgPath, err)
	}
}

func vprintln(a ...interface{}) {
	if *verbose {
		fmt.Fprintln(os.Stderr, a...)
	}
}

func getToken(ctx context.Context) *oauth2.Token {
	ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		log.Fatalf("google.DefaultTokenSource: %v", err)
	}
	tok, err := ts.Token()
	if err != nil {
		log.Fatalf("ts.Token: %v", err)
	}

	return tok
}

type gkeCluster struct {
	Endpoint   string
	MasterAuth struct {
		ClusterCaCertificate string
	}
	ResourceLabels map[string]string
}

func getCluster(ctx context.Context, tok *oauth2.Token) gkeCluster {
	url := fmt.Sprintf("https://container.googleapis.com/v1/projects/%s/locations/%s/clusters/%s", *project, *location, *clusterName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Fatalf("http.NewRequestWithContext: %v", err)
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
	target := gkeCluster{}
	if err := json.NewDecoder(resp.Body).Decode(&target); err != nil {
		log.Fatalf("json.Decode: %v", err)
	}

	return target
}

func checkClusterIsPrivileged(cluster gkeCluster) bool {
	if val, ok := cluster.ResourceLabels["privileged"]; ok && val == "true" {
		vprintln("configuring privileged cluster context")

		if checkContextExpired() {
			timeoutSeconds := 300
			if val, ok := cluster.ResourceLabels["timeout-seconds"]; ok {
				seconds, err := strconv.Atoi(val)
				if err != nil {
					log.Fatalf("strconv.Atoi: %v", err)
				}

				timeoutSeconds = seconds
			}

			vprintln(fmt.Sprintf("timeout reached, setting to %v seconds", timeoutSeconds))

			fmt.Fprintf(os.Stderr, "cluster %v/%v/%v is privileged, you will be re-prompted after %v seconds, proceed? [Y/n] ", *project, *location, *clusterName, timeoutSeconds)
			prompt := ""
			fmt.Scanln(&prompt)
			if prompt != "Y" {
				log.Fatalf("aborting access to privileged cluster %v/%v/%v", *project, *location, *clusterName)
			}

			timeout := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
			os.WriteFile(getExpFilename(), []byte(timeout.Format(time.Layout)), 0600)
		}

		return true
	}

	return false
}

func checkContextExpired() bool {
	exp := time.Time{}

	f, err := os.Open(getExpFilename())
	if err == nil {
		buf, err := io.ReadAll(f)
		if err != nil {
			log.Fatalf("io.ReadAll: %v", err)
		}

		e, err := time.Parse(time.Layout, string(buf))
		if err != nil {
			log.Fatalf("time.Parse: %v", err)
		}

		exp = e
		vprintln("overriding expiration timeout")
	} else {
		// if the file doesn't exist that just means it's the first time the context is being used
		// any other error is bad
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatalf("os.Open: %v", err)
		}

		vprintln("expiration file not found in /tmp, creating a new expiration")
	}

	return time.Now().After(exp)
}

func getExpFilename() string {
	return fmt.Sprintf("/tmp/gke-auth-privileged-timeout-%v-%v-%v", *project, *location, *clusterName)
}
