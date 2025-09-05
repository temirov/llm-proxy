SHELL := /bin/bash
CLASP := npx -y @google/clasp@2.5.0

define use_env
  cp -f .clasp.$(1).json .clasp.json
endef

login:
	$(CLASP) login

push-dev:
	@$(call use_env,dev)
	$(CLASP) push --force

push-prod:
	@$(call use_env,prod)
	$(CLASP) push --force

pull-dev:
	@$(call use_env,dev)
	$(CLASP) pull

pull-prod:
	@$(call use_env,prod)
	$(CLASP) pull

version-dev:
	@$(call use_env,dev)
	$(CLASP) version "bump"

version-prod:
	@$(call use_env,prod)
	$(CLASP) version "bump"

deploy-dev:
	@$(call use_env,dev)
	$(CLASP) deploy --description "dev"

deploy-prod:
	@$(call use_env,prod)
	$(CLASP) deploy --description "prod"

deployments-dev:
	@$(call use_env,dev)
	$(CLASP) deployments

deployments-prod:
	@$(call use_env,prod)
	$(CLASP) deployments
