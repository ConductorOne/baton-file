# `baton-file` Connector: Excel (`.xlsx`) Instructions

This document provides detailed instructions on how to structure your data within a Microsoft Excel (`.xlsx`) file for use with the `baton-file` connector.

## Overview

The connector expects an `.xlsx` file containing specific sheets (tabs) named `users`, `resources`, `entitlements`, and `grants`. Each sheet must have a header row defining the columns.

*   **Column Order:** The order of columns within a sheet does not matter.
*   **Header Case:** Standard header names (e.g., "Display Name", "Resource Type") are matched case-insensitively.
*   **Profile Headers:** Special `Profile: *` headers in the `users` sheet are case-sensitive *after* the "Profile: " prefix.
*   **Required Sheets:** While all four sheets are processed if present, the connector can function if some are missing (e.g., if you only have users and resources). However, grants require principals (users/resources) and entitlements to be defined.
*   **Template:** A template file is available at [`templates/template.xlsx`](../templates/template.xlsx).

## Sheet Definitions

### Sheet: `users`

**Purpose:** Defines all user principals, including regular users and service accounts. User data must *only* be defined in this sheet.

**Required Columns:**

*   `Name`: (Text) The unique identifier for the user within the system. This is used as the primary key and for linking in the `grants` sheet. *Example: `alice.admin`, `svc.data.agent`*
*   `Display Name`: (Text) The user's full name or display name shown in Baton/ConductorOne. *Example: `Alice Admin`, `Data Agent Service Acct`*

**Optional Columns:**

*   `Email`: (Text) The user's primary email address. *Example: `alice.admin@example.com`*
*   `Status`: (Text) The user's account status. Common values: `enabled`, `active`, `inactive`, `disabled`, `suspended`. If omitted or unrecognized, defaults to `enabled`. *Example: `enabled`, `disabled`*
*   `Type`: (Text) The type of user account. Common values: `human`, `user`, `person`, `service`, `system`, `bot`, `machine`. If omitted or unrecognized, defaults to `human`. *Example: `human`, `service`*
*   `Profile: *`: (Text) Any number of additional columns starting *exactly* with the prefix `Profile: ` (note the space). The text *after* this prefix becomes the key (case-sensitive) in the user's profile map in Baton. Values should be text. *Example Headers: `Profile: Department`, `Profile: Title`, `Profile: EmployeeID`*

**Example Row:**

| Name             | Display Name     | Email                      | Status   | Type    | Profile: Department | Profile: Title      |
| :--------------- | :--------------- | :------------------------- | :------- | :------ | :------------------ | :------------------ |
| `dave.developer` | `Dave Developer` | `dave.developer@example.com` | `active` | `human` | `Engineering`       | `Software Engineer` |

### Sheet: `resources`

**Purpose:** Defines all non-user resources (e.g., groups, roles, applications, workspaces) and assigns their primary Baton trait.

**Required Columns:**

*   `Resource Type`: (Text) The type name for this category of resource. Used internally and for display. Choose consistent names for related resources. *Example: `workspace`, `team`, `role`, `application`*
*   `Resource Function`: (Text) Defines the primary Baton trait for *all* resources of the corresponding `Resource Type`. Valid values (case-insensitive): `group`, `role`, `app`, `secret`. If a resource type should not have a specific trait, provide an empty value or a value not in the valid list (it will default to `TRAIT_UNSPECIFIED`). *Example: `group`, `role`*
*   `Name`: (Text) The unique identifier for this specific resource instance. Used as the primary key and for linking in `entitlements` and `grants`. *Example: `development_workspace`, `admins_team`, `billing_app_admin_role`*
*   `Display Name`: (Text) The human-readable name for this resource instance. *Example: `Development Workspace`, `Administrators Team`, `Billing App Admin Role`*

**Optional Columns:**

*   `Description`: (Text) A description for this resource instance. *Example: `Primary AWS development account`*
*   `Parent Resource`: (Text) The unique identifier (`Name`) of the parent resource. The parent must be defined in either the `users` or `resources` sheet. If omitted, the resource has no parent. *Example: `development_workspace`*

**Example Row:**

| Resource Type | Resource Function | Name          | Display Name  | Description                 | Parent Resource |
| :------------ | :---------------- | :------------ | :------------ | :-------------------------- | :-------------- |
| `team`        | `group`           | `app_dev_team` | `App Dev Team` | `Primary app development team` |                 |
| `role`        | `role`            | `dev_lead`    | `Dev Lead`    | `Development lead role`     | `app_dev_team`  |

### Sheet: `entitlements`

**Purpose:** Defines specific permissions, membership types, or role assignments (entitlements) that can be granted *on* resources defined in the `resources` sheet.

**Required Columns:**

*   `Resource Name`: (Text) The unique identifier (`Name`) of the resource (from the `resources` sheet) to which this entitlement applies. *Example: `admins_team`, `development_workspace`*
*   `Entitlement`: (Text) The specific entitlement *slug* (short identifier) being defined on the resource. This is used for linking in the `grants` sheet. *Example: `member`, `owner`, `admin`, `read`, `write`, `assigned`*
*   `Entitlement Display Name`: (Text) The human-readable name for the entitlement, often matching the slug or providing a more descriptive name. *Example: `Member`, `Owner`, `Admin Access`, `Assigned`*

**Optional Columns:**

*   `Entitlement Description`: (Text) A description of the entitlement. *Example: `Membership in the Admins team`*

**Example Row:**

| Resource Name | Entitlement | Entitlement Display Name | Entitlement Description          |
| :------------ | :---------- | :----------------------- | :------------------------------- |
| `app_dev_team` | `member`    | `Member`                 | `Membership in App Dev Team`   |
| `billing_app` | `admin`     | `Admin Access`           | `Full administrative privileges` |

### Sheet: `grants`

**Purpose:** Defines which principals (users or other resources like groups/roles) are granted which entitlements.

**Required Columns:**

*   `Principal Receiving Grant`: (Text) The unique identifier (`Name`) of the user (from `users`) or resource (from `resources`) receiving the grant. For group/role-based expansion, this can also be an entitlement key (see note below). *Example: `alice.admin`, `app_dev_team`, `dev_lead:assigned`*
*   `Entitlement Granted to Principal`: (Text) The full identifier of the entitlement being granted, in the format `resource_name:entitlement_slug` (matching data from the `entitlements` sheet). *Example: `app_dev_team:member`, `billing_app:admin`*

**Important Note on `Principal Receiving Grant` for Grant Expansion:**

*   **Direct Grant (No Expansion):** If you list the principal's `Name` (e.g., `alice.admin`, `app_dev_team`), a direct grant is created.
*   **Grant Expansion (Groups/Roles):** To leverage Baton's grant expansion feature (where granting access to a group/role implicitly grants it to its members/assignees), the `Principal Receiving Grant` *must* be the specific *entitlement key* that defines membership or assignment for that group/role. This key should be in the format `resource_name:entitlement_slug` (e.g., `app_dev_team:member`, `dev_lead:assigned`). Simply using the resource name (e.g., `app_dev_team`) will create the grant but *not* enable expansion based on its members/assignees.

**Example Row:**

| Principal Receiving Grant | Entitlement Granted to Principal |
| :------------------------ | :------------------------------- |
| `dave.developer`          | `app_dev_team:member`            |
| `app_dev_team:member`     | `billing_app:read`               | *<-- Grant to members of app_dev_team* 