package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
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
spec:
  parameters:
    storageGB: 20
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
          databaseVersion: POSTGRES_12
          region: us-central1
          settings:
            tier: db-custom-1-3840
            dataDiskType: PD_SSD
            ipConfiguration:
              ipv4Enabled: true
              authorizedNetworks:
                - value: "0.0.0.0/0"
    patches:
    - type: FromCompositeFieldPath
      fromFieldPath: spec.parameters.storageGB
      toFieldPath: spec.forProvider.settings.dataDiskSizeGb`

func serve() {
	tmpl := template.Must(template.ParseFS(resources, "templates/forms.html"))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		type Response struct {
			POST              bool
			CompositeResource string
			Composition       string
			Response          string
			Error             error
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

		resp.Response, resp.Error = eval(resp.CompositeResource, resp.Composition)
		err := tmpl.Execute(w, resp)
		if err != nil {
			log.Println(err)
		}
	})

	http.ListenAndServe(":8080", nil)
}
