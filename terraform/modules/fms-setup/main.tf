variable "create_placeholder" {
  type    = bool
  default = true
}

# This module is intentionally minimal; it represents "Org-level FMS setup"
# which typically happens once and is not managed by this PoC.
#
# AI EXTENSION: Replace this placeholder with:
# - Data lookups for current FMS admin
# - Validation that Organizations + FMS are enabled

resource "null_resource" "placeholder" {
  count = var.create_placeholder ? 1 : 0

  provisioner "local-exec" {
    command = "echo 'FMS admin setup is assumed to be done outside of this PoC.'"
  }
}
