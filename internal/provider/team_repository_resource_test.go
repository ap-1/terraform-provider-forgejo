package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccTeamRepositoryResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing (non-existent team)
			{
				Config: providerConfig + `
resource "forgejo_organization" "test" {
	name = "test_org"
}
resource "forgejo_repository" "test" {
	owner = forgejo_organization.test.name
	name  = "test_repo"
}
resource "forgejo_team_repository" "test" {
	team_id       = 1010
	repository_id = forgejo_repository.test.id
}`,
				ExpectError: regexp.MustCompile("Either repository '.*' or team with ID 1010 not found"),
			},
			// Create and Read testing (non-existent repository)
			{
				Config: providerConfig + `
resource "forgejo_organization" "test" {
	name = "test_org"
}
resource "forgejo_team" "test" {
	name            = "test_team"
	organization_id = forgejo_organization.test.id
	permission      = "read"
	units_map       = {
		"repo.code" = "read"
	}
}
resource "forgejo_team_repository" "test" {
	team_id       = forgejo_team.test.id
	repository_id = 9999
}`,
				ExpectError: regexp.MustCompile("Repository with ID 9999 not found"),
			},
			// Create and Read testing
			{
				Config: providerConfig + `
resource "forgejo_organization" "test" {
	name = "test_org"
}
resource "forgejo_team" "test" {
	name            = "test_team"
	organization_id = forgejo_organization.test.id
	permission      = "read"
	units_map       = {
		"repo.code" = "read"
	}
}
resource "forgejo_repository" "test" {
	owner = forgejo_organization.test.name
	name  = "test_repo"
}
resource "forgejo_team_repository" "test" {
	team_id       = forgejo_team.test.id
	repository_id = forgejo_repository.test.id
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("forgejo_team_repository.test", plancheck.ResourceActionCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("forgejo_team_repository.test", tfjsonpath.New("team_id"), "forgejo_team.test", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("forgejo_team_repository.test", tfjsonpath.New("repository_id"), "forgejo_repository.test", tfjsonpath.New("id"), compare.ValuesSame()),
				},
			},
			// Recreate and Read testing (updating any value recreates the resource -- repository)
			{
				Config: providerConfig + `
resource "forgejo_organization" "test" {
	name = "test_org"
}
resource "forgejo_team" "test" {
	name            = "test_team"
	organization_id = forgejo_organization.test.id
	permission      = "read"
	units_map       = {
		"repo.code" = "read"
	}
}
resource "forgejo_repository" "test" {
	owner = forgejo_organization.test.name
	name  = "test_repo"
}
resource "forgejo_repository" "test2" {
	owner = forgejo_organization.test.name
	name  = "second_test_repo"
}
resource "forgejo_team_repository" "test" {
	team_id       = forgejo_team.test.id
	repository_id = forgejo_repository.test2.id
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("forgejo_team_repository.test", plancheck.ResourceActionReplace),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("forgejo_team_repository.test", tfjsonpath.New("team_id"), "forgejo_team.test", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("forgejo_team_repository.test", tfjsonpath.New("repository_id"), "forgejo_repository.test2", tfjsonpath.New("id"), compare.ValuesSame()),
				},
			},
			// Recreate and Read testing (updating any value recreates the resource -- team)
			{
				Config: providerConfig + `
resource "forgejo_organization" "test" {
	name = "test_org"
}
resource "forgejo_team" "test" {
	name            = "test_team"
	organization_id = forgejo_organization.test.id
	permission      = "read"
	units_map       = {
		"repo.code" = "read"
	}
}
resource "forgejo_team" "test2" {
	name            = "second_test_team"
	organization_id = forgejo_organization.test.id
	permission      = "read"
	units_map       = {
		"repo.code" = "read"
	}
}
resource "forgejo_repository" "test2" {
	owner = forgejo_organization.test.name
	name  = "second_test_repo"
}
resource "forgejo_team_repository" "test" {
	team_id       = forgejo_team.test2.id
	repository_id = forgejo_repository.test2.id
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("forgejo_team_repository.test", plancheck.ResourceActionReplace),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs("forgejo_team_repository.test", tfjsonpath.New("team_id"), "forgejo_team.test2", tfjsonpath.New("id"), compare.ValuesSame()),
					statecheck.CompareValuePairs("forgejo_team_repository.test", tfjsonpath.New("repository_id"), "forgejo_repository.test2", tfjsonpath.New("id"), compare.ValuesSame()),
				},
			},
			// Import testing
			{
				ResourceName:                         "forgejo_team_repository.test",
				ImportState:                          true,
				ImportStateId:                        "test_org/second_test_team/second_test_repo",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "repository_id",
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
