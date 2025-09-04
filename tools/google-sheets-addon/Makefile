SHELL := /bin/bash
CLASP := npx clasp
ROOT  := src

# Copy the selected env file into .clasp.json for clasp to use.
define use_env
  cp -f .clasp.$(1).json .clasp.json
endef

login:
	$(CLASP) login

push-dev:
	@$(call use_env,dev)
	$(CLASP) push --rootDir $(ROOT) --force

push-prod:
	@$(call use_env,prod)
	$(CLASP) push --rootDir $(ROOT) --force

pull-dev:
	@$(call use_env,dev)
	$(CLASP) pull --rootDir $(ROOT)

pull-prod:
	@$(call use_env,prod)
	$(CLASP) pull --rootDir $(ROOT)

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
