package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
)

//go:embed templates/*
var resources embed.FS

// bootstrap some files for the user
const cr = `apiVersion: database.example.org/v1alpha1
kind: XPostgreSQLInstance
metadata:
  name: my-db
  labels:
    crossplane.io/composite: xpostgresqlinstances.database.example.org
    crossplane.io/claim-name: my-db
    crossplane.io/claim-namespace: my-ns
  uid: "d9d89470-4d02-4964-9d85-7287976bf5bd"
spec:
  parameters:
    storageGB: 20
    #storageGBMax: 30
    size: s
    mail: foo@example.com
  compositionRef:
    name: production
  writeConnectionSecretToRef:
    namespace: crossplane-system
    name: my-db-connection-details`

const composition = `apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: example
spec:
  writeConnectionSecretsToNamespace: crossplane-system
  compositeTypeRef:
    apiVersion: database.example.org/v1alpha1
    kind: XPostgreSQLInstance
  resources:
  - name: cloudsqlinstance
    base:
      apiVersion: database.gcp.crossplane.io/v1beta1
      kind: CloudSQLInstance
      spec:
        forProvider:
          region: us-central1
    patches:
    - type: FromCompositeFieldPath
      fromFieldPath: spec.parameters.storageGB
      toFieldPath: spec.forProvider.dataDiskSizeGb

    # Set dataDiskSizeGbMax to 3x storageGB if storageGBMax is not defined
    - type: FromCompositeFieldPath
      fromFieldPath: spec.parameters.storageGB
      toFieldPath: spec.forProvider.dataDiskSizeGbMax
      transforms:
      - type: math
        math:
          multiply: 3
    - type: FromCompositeFieldPath
      fromFieldPath: spec.parameters.storageGBMax
      toFieldPath: spec.forProvider.dataDiskSizeGbMax
      policy:
        fromFieldPath: Optional # Optional is default, which means this patch is skipped and the 3x patch above is used if storageGBMax is missing

# Convert to string for map lookup
#    - type: FromCompositeFieldPath
#      fromFieldPath: spec.parameters.storageGB
#      toFieldPath: spec.forProvider.dataDiskSizeHuman
#      transforms:
#      - type: string
#        string:
#         type: Format
#         fmt: "%d"
#      - type: map
#        map:
#         "20": "twenty"

    - type: FromCompositeFieldPath
      fromFieldPath: spec.parameters.size
      toFieldPath: spec.forProvider.size
      transforms:
      - type: map
        map:
         s: "small"
         m: "medium"

    - type: FromCompositeFieldPath
      fromFieldPath: spec.parameters.mail
      toFieldPath: spec.forProvider.emailUser
      transforms:
      - type: string
        string:
         type: Regexp
         regexp:
           match: "(.*)@.*"
           group: 1
      - type: string
        string:
          type: Convert
          convert: ToUpper

    - type: FromCompositeFieldPath
      fromFieldPath: spec.parameters.mail
      toFieldPath: spec.forProvider.emailBase64
      transforms:
      - type: string
        string:
          type: Convert
          convert: ToBase64`

var (
	tmpl = template.Must(template.ParseFS(resources, "templates/forms.html"))
)

func handler(w http.ResponseWriter, r *http.Request) {
	type Response struct {
		POST              bool
		CompositeResource string
		Composition       string
		Response          string
		Error             string
	}
	if r.Method != http.MethodPost {
		err := tmpl.Execute(w, Response{CompositeResource: cr, Composition: composition})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	resp := Response{
		CompositeResource: r.FormValue("compositeResource"),
		Composition:       r.FormValue("composition"),
		POST:              true,
	}

	var err error
	resp.Response, err = eval(resp.CompositeResource, resp.Composition)
	if err != nil {
		resp.Error = err.Error()
		log.Println(err)
	}

	if r.Header.Get("Accept") == "application/json" {
		err = json.NewEncoder(w).Encode(resp)
	} else {
		err = tmpl.Execute(w, resp)
	}
	if err != nil {
		log.Println(err)
	}
}

func serve() {
	http.Handle("/", handlers.LoggingHandler(os.Stdout, http.HandlerFunc(handler)))

	http.ListenAndServe(":8080", nil)
}
