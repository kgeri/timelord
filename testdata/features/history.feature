Feature: Usage history
  The admin console charts daily usage for one user.

  Background:
    Given a Timetower server with configuration "config/valid.json" at "2024-06-05T10:00:00Z"

  Scenario: History defaults to seven zero-filled days
    When the admin requests the usage history for "dani"
    Then the response status is 200
    And the usage history is:
      | user  | dani         |
      | entry | 2024-05-30 0 |
      | entry | 2024-05-31 0 |
      | entry | 2024-06-01 0 |
      | entry | 2024-06-02 0 |
      | entry | 2024-06-03 0 |
      | entry | 2024-06-04 0 |
      | entry | 2024-06-05 0 |

  Scenario: History aggregates usage across hostnames and fills missing days
    When the enforcer reports:
      | hostname | pc-one     |
      | day      | 2024-06-03 |
      | used     | 1000       |
    And the enforcer reports:
      | hostname | pc-two     |
      | day      | 2024-06-03 |
      | used     | 500        |
    And the enforcer reports:
      | day  | 2024-06-05 |
      | used | 1200       |
    And the admin requests the usage history for "dani"
    Then the usage history is:
      | entry | 2024-05-30 0    |
      | entry | 2024-05-31 0    |
      | entry | 2024-06-01 0    |
      | entry | 2024-06-02 0    |
      | entry | 2024-06-03 1500 |
      | entry | 2024-06-04 0    |
      | entry | 2024-06-05 1200 |

  Scenario: The admin selects the number of days
    When the admin requests the usage history for "dani" with days "3"
    Then the usage history is:
      | entry | 2024-06-03 0 |
      | entry | 2024-06-04 0 |
      | entry | 2024-06-05 0 |

  Scenario Outline: Invalid day counts are rejected
    When the admin requests the usage history for "dani" with days "<days>"
    Then the response status is 400

    Examples:
      | days |
      | 0    |
      | 401  |
      | nope |

  Scenario: The history requires admin authentication
    When the usage history is requested without credentials
    Then the response status is 401

  Scenario: Enforcer credentials cannot read history
    When the enforcer requests the usage history with its key
    Then the response status is 403

  Scenario: Unknown users have no history
    When the admin requests the usage history for "other"
    Then the response status is 404

  Scenario: History only accepts GET
    When the admin sends a POST to the usage history
    Then the response status is 405
    And the Allow header is "GET"
