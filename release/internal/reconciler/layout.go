package reconciler

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// DesiredStateLayout documents the expected desired-state repo structure:
//
//	desired-state/
//	  <region>/
//	    <project>/
//	      deployment.yaml
//	      service.yaml
//	      (any additional k8s manifests)
//
// WriteManifests generates a minimal Deployment + Service for imageTag
// and writes them under baseDir/<region>/<project>/.
func WriteManifests(baseDir, region, project, imageTag string) error {
	dir := filepath.Join(baseDir, region, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}

	depData := struct {
		Project  string
		ImageTag string
	}{Project: project, ImageTag: imageTag}

	if err := renderTemplate(filepath.Join(dir, "deployment.yaml"), deploymentTmpl, depData); err != nil {
		return fmt.Errorf("write deployment.yaml: %w", err)
	}

	svcData := struct{ Project string }{Project: project}
	if err := renderTemplate(filepath.Join(dir, "service.yaml"), serviceTmpl, svcData); err != nil {
		return fmt.Errorf("write service.yaml: %w", err)
	}

	return nil
}

func renderTemplate(path, tmplStr string, data any) error {
	tmpl, err := template.New("").Parse(tmplStr)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

const deploymentTmpl = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Project }}
  labels:
    app: {{ .Project }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .Project }}
  template:
    metadata:
      labels:
        app: {{ .Project }}
    spec:
      containers:
        - name: {{ .Project }}
          image: {{ .ImageTag }}
          ports:
            - containerPort: 8080
          readinessProbe:
            httpGet:
              path: /actuator/health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
`

const serviceTmpl = `apiVersion: v1
kind: Service
metadata:
  name: {{ .Project }}
spec:
  selector:
    app: {{ .Project }}
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
`
