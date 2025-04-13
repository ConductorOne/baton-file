# `baton-file` Connector: JSON (`.json`) Instructions

This document provides detailed instructions on how to structure your data within a JSON (`.json`) file for use with the `baton-file` connector.

## Overview

The connector expects a JSON file containing a single top-level object. This object must have four keys: `users`, `resources`, `entitlements`, and `grants`. Each key should hold an array of objects.

*   **Keys:** Object keys within the arrays must match the expected field names defined below (lowercase snake_case). Key order generally does not matter in JSON objects.
*   **Required Sections:** While all four top-level keys are processed if present, the connector can function if some arrays are empty or keys are missing (e.g., if you only have users and resources). However, grants require principals (users/resources) and entitlements to be defined.
*   **Template:** A template file is available at [`templates/template.json`](../templates/template.json).

## Top-Level Keys & Data Structure

### Key: `users`

**Purpose:** Defines all user principals, including regular users and service accounts.
**Format:** An array of user objects.

**User Object Fields:**

*   `name`: (String, **Required**) The unique identifier for the user. *Example: `"alice.admin"`, `"svc.data.agent"`*
*   `display_name`: (String, **Required**) The user's full name or display name. *Example: `"Alice Admin"`, `"Data Agent Service Acct"`*
*   `email`: (String, Optional) The user's primary email address. *Example: `"alice.admin@example.com"`*
*   `status`: (String, Optional) The user's account status. Common values: `"enabled"`, `"active"`, `"inactive"`, `"disabled"`, `"suspended"`. Defaults to `enabled`. *Example: `"active"`, `"inactive"`*
*   `type`: (String, Optional) The type of user account. Common values: `"human"`, `"user"`, `"person"`, `"service"`, `"system"`, `"bot"`, `"machine"`. Defaults to `human`. *Example: `"human"`, `"service"`*
*   `profile`: (Object, Optional) An object containing additional user profile attributes. Keys should be strings, values can be strings, numbers, or booleans. *Example: `{ "department": "Engineering", "title": "Software Engineer", "employee_id": 12345 }`*

**Example:**
```json
{
  "users": [
    {
      "name": "dave.developer",
      "display_name": "Dave Developer",
      "email": "dave.developer@example.com",
      "status": "active",
      "type": "human",
      "profile": {
        "department": "Engineering",
        "title": "Software Engineer",
        "hire_date": "2025-01-02"
      }
    },
    {
      "name": "svc.account.01",
      "display_name": "Service Account 01",
      "email": "svc.account.01@example.com",
      "status": "active",
      "type": "service",
      "profile": {}
    }
  ],
  "resources": [],
  "entitlements": [],
  "grants": []
}
```

### Key: `resources`

**Purpose:** Defines all non-user resources (e.g., groups, roles, applications, workspaces) and assigns their primary Baton trait.
**Format:** An array of resource objects.

**Resource Object Fields:**

*   `resource_type`: (String, **Required**) The type name for this category of resource. *Example: `"workspace"`, `"team"`, `"role"`, `"application"`*
*   `resource_function`: (String, **Required**) Defines the primary Baton trait. Valid values: `"group"`, `"role"`, `"app"`, `"secret"`. Use an empty string `""` or a non-matching value for no specific trait. *Example: `"group"`, `"role"`*
*   `name`: (String, **Required**) The unique identifier for this resource instance. *Example: `"development_workspace"`, `"admins_team"`, `"billing_app_admin_role"`*
*   `display_name`: (String, **Required**) The human-readable name. *Example: `"Development Workspace"`, `"Administrators Team"`*
*   `description`: (String, Optional) A description for this resource. *Example: `"Primary AWS development account"`*
*   `parent_resource`: (String, Optional) The unique identifier (`name`) of the parent resource (must be a user or another resource). Use an empty string `""` or omit/`null` for no parent. *Example: `"development_workspace"`*

**Example:**
```json
  "resources": [
    {
      "resource_type": "team",
      "resource_function": "group",
      "name": "app_dev_team",
      "display_name": "App Dev Team",
      "description": "Primary app development team",
      "parent_resource": ""
    },
    {
      "resource_type": "role",
      "resource_function": "role",
      "name": "dev_lead",
      "display_name": "Dev Lead",
      "description": "Development lead role",
      "parent_resource": "app_dev_team"
    }
  ]
```

### Key: `entitlements`

**Purpose:** Defines specific permissions, membership types, or role assignments (entitlements) on resources.
**Format:** An array of entitlement objects.

**Entitlement Object Fields:**

*   `resource_name`: (String, **Required**) The unique identifier (`name`) of the resource this entitlement applies to. *Example: `"admins_team"`, `"development_workspace"`*
*   `entitlement`: (String, **Required**) The specific entitlement *slug*. *Example: `"member"`, `"owner"`, `"admin"`, `"read"`, `"assigned"`*
*   `display_name`: (String, **Required**) The human-readable name. *Example: `"Member"`, `"Owner"`, `"Admin Access"`*
*   `description`: (String, Optional) A description. *Example: `"Membership in the Admins team"`*

**Example:**
```json
  "entitlements": [
    {
      "resource_name": "app_dev_team",
      "entitlement": "member",
      "display_name": "Member",
      "description": "Membership in App Dev Team"
    },
    {
      "resource_name": "billing_app",
      "entitlement": "admin",
      "display_name": "Admin Access",
      "description": "Full administrative privileges"
    }
  ]
```

### Key: `grants`

**Purpose:** Defines which principals are granted which entitlements.
**Format:** An array of grant objects.

**Grant Object Fields:**

*   `principal`: (String, **Required**) The unique identifier (`name`) of the user or resource receiving the grant. Can also be an entitlement key for expansion (see note below). *Example: `"alice.admin"`, `"app_dev_team"`, `"dev_lead:assigned"`*
*   `entitlement_id`: (String, **Required**) The full identifier of the entitlement being granted (`resource_name:entitlement_slug`). *Example: `"app_dev_team:member"`, `"billing_app:admin"`*

**Important Note on `principal` for Grant Expansion:**

*   **Direct Grant (No Expansion):** List the principal's `name` (e.g., `"alice.admin"`, `"app_dev_team"`).
*   **Grant Expansion (Groups/Roles):** Use the specific *entitlement key* defining membership/assignment (`resource_name:entitlement_slug`, e.g., `"app_dev_team:member"`, `"dev_lead:assigned"`). Using only the resource name (e.g., `"app_dev_team"`) will *not* enable expansion.

**Example:**
```json
  "grants": [
    {
      "principal": "dave.developer",
      "entitlement_id": "app_dev_team:member"
    },
    {
      "principal": "app_dev_team:member", // Grant to members of app_dev_team
      "entitlement_id": "billing_app:read"
    }
  ]
``` 