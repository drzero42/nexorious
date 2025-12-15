# IGDB API Documentation

> Source: [api-docs.igdb.com](https://api-docs.igdb.com/) (downloaded 2025-12-15)

---

## Account Creation

Requirements:
1. Sign Up with [Twitch](https://dev.twitch.tv/login)
2. Enable [Two Factor Authentication](https://www.twitch.tv/settings/security)
3. Register application in the [Twitch Developer Portal](https://dev.twitch.tv/console/apps/create)
   - OAuth Redirect URL: use `localhost`
   - Client Type: must be `Confidential`
4. [Manage](https://dev.twitch.tv/console/apps) your application and generate a Client Secret
5. Save your Client ID and Client Secret

License: Free for **non-commercial** use per [Twitch Developer Service Agreement](https://www.twitch.tv/p/legal/developer-agreement/).

## Authentication

Authenticate via Twitch OAuth2. See [Twitch Developer Docs](https://dev.twitch.tv/docs/authentication).

```
POST https://id.twitch.tv/oauth2/token?client_id={CLIENT_ID}&client_secret={CLIENT_SECRET}&grant_type=client_credentials
```

Response:
```json
{
 "access_token": "access12345token",
 "expires_in": 5587808,
 "token_type": "bearer"
}
```

The `expires_in` field shows seconds until token expiration.


## Requests

- Method: `POST`
- Base URL: `https://api.igdb.com/v4`
- Endpoint: append `/{endpoint}` (e.g., `https://api.igdb.com/v4/games`)
- Headers (note capitalization):
  ```
  Client-ID: {CLIENT_ID}
  Authorization: Bearer {ACCESS_TOKEN}
  ```
- Body: query string specifying fields, filters, sorting, etc.

Example:
```
POST https://api.igdb.com/v4/games
Client-ID: abcdefg12345
Authorization: Bearer access12345token
Body: "fields *;"
```

## Rate Limits

- **4 requests per second** - exceeding returns `429 Too Many Requests`
- Maximum 8 concurrent open requests

## Wrappers

- [Python](https://github.com/twitchtv/igdb-api-python)
- [NodeJS](https://github.com/igdb/node-apicalypse)

## Examples

### Get 10 game names

```
fields name; limit 10;
```

### Get specific game by ID
```
fields *; where id = 1942;
```

### Exclude fields
```
fields *; exclude alternative_name;
```

### Filter by multiple IDs (use parentheses)
```
fields *; where id = (8,9,11);
```

### Count with filter
```
where rating > 75;  # Use /games/count endpoint
```

### Sort by field
```
fields name,rating; sort rating desc;
```

### Combined filters (& = AND, | = OR)
```
fields *; where platforms = 48 & date > 1538129354; sort date asc;
```

### Search with fields
```
search "Halo"; fields name,release_dates.human;
```

### Search excluding editions
```
search "Assassins Creed"; fields name; where version_parent = null;
```


### Platform exclusives
```
fields name,platforms; where platforms = 48;  # PS4 only
```

### Exact platform match (PS4 AND PC only)
```
fields name,platforms; where platforms = {48,6};
```

---

# Endpoints

All endpoints use `POST` to `https://api.igdb.com/v4/{endpoint}` with query in body.

## Age Rating

`/age_ratings`

| field | type | description |
| --- | --- | --- |
| organization | Reference ID | Rating organization (ESRB, PEGI, etc.) |
| rating_category | Reference ID | The rating category |
| rating_cover_url | String | URL for rating image |
| synopsis | String | Rating description |


## Age Rating Content Description


```python
from requests import post
response = post('https://api.igdb.com/v4/age_rating_content_descriptions', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,description;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:

category: DEPRECATED!

DEPRECATED! Use `age_rating_content_descriptions_v2` instead


### Request Path

`https://api.igdb.com/v4/age_rating_content_descriptions`


| field | type | description |
| --- | --- | --- |
| category | Category Enum | DEPRECATED! |
| checksum | uuid | Hash of the object |
| description | String | |


### Age Rating Content Description Enums

category


| name | value |
| --- | --- |
| ESRB_alcohol_reference | 1 |
| ESRB_animated_blood | 2 |
| ESRB_blood | 3 |
| ESRB_blood_and gore | 4 |
| ESRB_cartoon_violence | 5 |
| ESRB_comic_mischief | 6 |
| ESRB_crude_humor | 7 |
| ESRB_drug_reference | 8 |
| ESRB_fantasy_violence | 9 |
| ESRB_intense_violence | 10 |
| ESRB_language | 11 |
| ESRB_lyrics | 12 |
| ESRB_mature_humor | 13 |
| ESRB_nudity | 14 |
| ESRB_partial_nudity | 15 |
| ESRB_real_gambling | 16 |
| ESRB_sexual_content | 17 |
| ESRB_sexual_themes | 18 |
| ESRB_sexual_violence | 19 |
| ESRB_simulated_gambling | 20 |
| ESRB_strong_language | 21 |
| ESRB_strong_lyrics | 22 |
| ESRB_strong_sexual content | 23 |
| ESRB_suggestive_themes | 24 |
| ESRB_tobacco_reference | 25 |
| ESRB_use_of alcohol | 26 |
| ESRB_use_of drugs | 27 |
| ESRB_use_of tobacco | 28 |
| ESRB_violence | 29 |
| ESRB_violent_references | 30 |
| ESRB_animated_violence | 31 |
| ESRB_mild_language | 32 |
| ESRB_mild_violence | 33 |
| ESRB_use_of drugs and alcohol | 34 |
| ESRB_drug_and alcohol reference | 35 |
| ESRB_mild_suggestive themes | 36 |
| ESRB_mild_cartoon violence | 37 |
| ESRB_mild_blood | 38 |
| ESRB_realistic_blood and gore | 39 |
| ESRB_realistic_violence | 40 |
| ESRB_alcohol_and tobacco reference | 41 |
| ESRB_mature_sexual themes | 42 |
| ESRB_mild_animated violence | 43 |
| ESRB_mild_sexual themes | 44 |
| ESRB_use_of alcohol and tobacco | 45 |
| ESRB_animated_blood and gore | 46 |
| ESRB_mild_fantasy violence | 47 |
| ESRB_mild_lyrics | 48 |
| ESRB_realistic_blood | 49 |
| PEGI_violence | 50 |
| PEGI_sex | 51 |
| PEGI_drugs | 52 |
| PEGI_fear | 53 |
| PEGI_discrimination | 54 |
| PEGI_bad_language | 55 |
| PEGI_gambling | 56 |
| PEGI_online_gameplay | 57 |
| PEGI_in_game_purchases | 58 |
| CERO_love | 59 |
| CERO_sexual_content | 60 |
| CERO_violence | 61 |
| CERO_horror | 62 |
| CERO_drinking_smoking | 63 |
| CERO_gambling | 64 |
| CERO_crime | 65 |
| CERO_controlled_substances | 66 |
| CERO_languages_and others | 67 |
| GRAC_sexuality | 68 |
| GRAC_violence | 69 |
| GRAC_fear_horror_threatening | 70 |
| GRAC_language | 71 |
| GRAC_alcohol_tobacco_drug | 72 |
| GRAC_crime_anti_social | 73 |
| GRAC_gambling | 74 |
| CLASS_IND_violencia | 75 |
| CLASS_IND_violencia_extrema | 76 |
| CLASS_IND_conteudo_sexual | 77 |
| CLASS_IND_nudez | 78 |
| CLASS_IND_sexo | 79 |
| CLASS_IND_sexo_explicito | 80 |
| CLASS_IND_drogas | 81 |
| CLASS_IND_drogas_licitas | 82 |
| CLASS_IND_drogas_ilicitas | 83 |
| CLASS_IND_linguagem_impropria | 84 |
| CLASS_IND_atos_criminosos | 85 |


## Age Rating Content Description Type


```python
from requests import post
response = post('https://api.igdb.com/v4/age_rating_content_description_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,slug,updated_at;'})
print ("response: %s" % str(response.json()))
```

Age Rating Content Description Types


### Request Path

`https://api.igdb.com/v4/age_rating_content_description_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Age Rating Content Description V2


```python
from requests import post
response = post('https://api.igdb.com/v4/age_rating_content_descriptions_v2', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,description,description_type,organization,updated_at;'})
print ("response: %s" % str(response.json()))
```

Age Rating Content Descriptions


### Request Path

`https://api.igdb.com/v4/age_rating_content_descriptions_v2`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | |
| description_type | Reference ID forAge Rating Content Description Type | The age rating content description type |
| organization | Reference ID forAge Rating Organization | The rating organization |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Age Rating Organization


```python
from requests import post
response = post('https://api.igdb.com/v4/age_rating_organizations', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Age Rating according to various rating organisations


### Request Path

`https://api.igdb.com/v4/age_rating_organizations`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | The title of an age rating organization |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Artwork


```python
from requests import post
response = post('https://api.igdb.com/v4/artworks', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alpha_channel,animated,artwork_type,checksum,game,height,image_id,url,width;'})
print ("response: %s" % str(response.json()))
```

Official artworks (resolution and aspect ratio may vary)


### Request Path

`https://api.igdb.com/v4/artworks`


| field | type | description |
| --- | --- | --- |
| alpha_channel | boolean | |
| animated | boolean | |
| artwork_type | Reference ID forArtwork Type | The artwork type |
| checksum | uuid | Hash of the object |
| game | Reference ID forGame | The game this artwork is associated with |
| height | Integer | The height of the image in pixels |
| image_id | String | The ID of the image used to construct an IGDB image link |
| url | String | The website address (URL) of the item |
| width | Integer | The width of the image in pixels |


## Alternative Name


```python
from requests import post
response = post('https://api.igdb.com/v4/alternative_names', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,comment,game,name;'})
print ("response: %s" % str(response.json()))
```

Alternative and international game titles


### Request Path

`https://api.igdb.com/v4/alternative_names`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| comment | String | A description of what kind of alternative name it is (Acronym, Working title, Japanese title etc) |
| game | Reference ID forGame | The game this alternative name is associated with |
| name | String | An alternative name |


## Artwork Type


```python
from requests import post
response = post('https://api.igdb.com/v4/artwork_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,slug,updated_at;'})
print ("response: %s" % str(response.json()))
```

Artwork Types


### Request Path

`https://api.igdb.com/v4/artwork_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Character


```python
from requests import post
response = post('https://api.igdb.com/v4/characters', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields akas,character_gender,character_species,checksum,country_name,created_at,description,games,gender,mug_shot,name,slug,species,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:


gender: DEPRECATED! Use character_gender instead


species: DEPRECATED! Use character_species instead

Video game characters


### Request Path

`https://api.igdb.com/v4/characters`


| field | type | description |
| --- | --- | --- |
| akas | Array of Strings | Alternative names for a character |
| character_gender | Reference ID forCharacter Gender | |
| character_species | Reference ID forCharacter Specie | |
| checksum | uuid | Hash of the object |
| country_name | String | A characters country of origin |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | A text describing a character |
| games | Array ofGameIDs | |
| gender | Gender Enum | DEPRECATED! Usecharacter_genderinstead |
| mug_shot | Reference ID forCharacter Mug Shot | An image depicting a character |
| name | String | |
| slug | String | A url-safe, unique, lower-case version of the name |
| species | Species Enum | DEPRECATED! Usecharacter_speciesinstead |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


### Character Enums

gender


| name | value |
| --- | --- |
| Male | 0 |
| Female | 1 |
| Other | 2 |

species


| name | value |
| --- | --- |
| Human | 1 |
| Alien | 2 |
| Animal | 3 |
| Android | 4 |
| Unknown | 5 |


## Character Gender


```python
from requests import post
response = post('https://api.igdb.com/v4/character_genders', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Character Genders


### Request Path

`https://api.igdb.com/v4/character_genders`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Character Mug Shot


```python
from requests import post
response = post('https://api.igdb.com/v4/character_mug_shots', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alpha_channel,animated,checksum,height,image_id,url,width;'})
print ("response: %s" % str(response.json()))
```

Images depicting game characters


### Request Path

`https://api.igdb.com/v4/character_mug_shots`


| field | type | description |
| --- | --- | --- |
| alpha_channel | boolean | |
| animated | boolean | |
| checksum | uuid | Hash of the object |
| height | Integer | The height of the image in pixels |
| image_id | String | The ID of the image used to construct an IGDB image link |
| url | String | The website address (URL) of the item |
| width | Integer | The width of the image in pixels |


## Character Specie


```python
from requests import post
response = post('https://api.igdb.com/v4/character_species', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Character Species


### Request Path

`https://api.igdb.com/v4/character_species`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Collection


```python
from requests import post
response = post('https://api.igdb.com/v4/collections', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields as_child_relations,as_parent_relations,checksum,created_at,games,name,slug,type,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Collection, AKA Series


### Request Path

`https://api.igdb.com/v4/collections`


| field | type | description |
| --- | --- | --- |
| as_child_relations | Array ofCollection RelationIDs | |
| as_parent_relations | Array ofCollection RelationIDs | |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| games | Array ofGameIDs | The games that are associated with this collection |
| name | String | Umbrella term for a collection of games |
| slug | String | A url-safe, unique, lower-case version of the name |
| type | Reference ID forCollection Type | The type of collection |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Collection Membership


```python
from requests import post
response = post('https://api.igdb.com/v4/collection_memberships', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,collection,created_at,game,type,updated_at;'})
print ("response: %s" % str(response.json()))
```

The Collection Memberships.


### Request Path

`https://api.igdb.com/v4/collection_memberships`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| collection | Reference ID forCollection | The collection that is associated with this membership |
| created_at | datetime | Date this was initially added to the IGDB database |
| game | Reference ID forGame | The game that is associated with this membership |
| type | Reference ID forCollection Membership Type | The Collection Membership Type |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Collection Membership Type


```python
from requests import post
response = post('https://api.igdb.com/v4/collection_membership_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields allowed_collection_type,checksum,created_at,description,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Enums for collection membership types.


### Request Path

`https://api.igdb.com/v4/collection_membership_types`


| field | type | description |
| --- | --- | --- |
| allowed_collection_type | Reference ID forCollection Type | The allowed collection type |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | Description of the membership type. |
| name | String | The membership type name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Collection Relation


```python
from requests import post
response = post('https://api.igdb.com/v4/collection_relations', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,child_collection,created_at,parent_collection,type,updated_at;'})
print ("response: %s" % str(response.json()))
```

Describes Relationship between Collections.


### Request Path

`https://api.igdb.com/v4/collection_relations`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| child_collection | Reference ID forCollection | The child collection of this collection. |
| created_at | datetime | Date this was initially added to the IGDB database |
| parent_collection | Reference ID forCollection | The parent collection of this collection. |
| type | Reference ID forCollection Relation Type | The collection relationship type |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Collection Relation Type


```python
from requests import post
response = post('https://api.igdb.com/v4/collection_relation_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields allowed_child_type,allowed_parent_type,checksum,created_at,description,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Collection Relation Types


### Request Path

`https://api.igdb.com/v4/collection_relation_types`


| field | type | description |
| --- | --- | --- |
| allowed_child_type | Reference ID forCollection Type | The allowed child collection type |
| allowed_parent_type | Reference ID forCollection Type | The allowed child collection type |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | The relationship type description |
| name | String | The relationship type name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Collection Type


```python
from requests import post
response = post('https://api.igdb.com/v4/collection_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,description,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Enums for collection types.


### Request Path

`https://api.igdb.com/v4/collection_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | Description of the collection type. |
| name | String | The name of the collection type |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Company


```python
from requests import post
response = post('https://api.igdb.com/v4/companies', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields change_date,change_date_category,change_date_format,changed_company_id,checksum,country,created_at,description,developed,logo,name,parent,published,slug,start_date,start_date_category,start_date_format,status,updated_at,url,websites;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:


change_date_category: DEPRECATED! Use change_date_format instead


start_date_category: DEPRECATED! Use start_date_format instead

Video game companies. Both publishers & developers


### Request Path

`https://api.igdb.com/v4/companies`


| field | type | description |
| --- | --- | --- |
| change_date | Unix Time Stamp | The data when a company got a new ID |
| change_date_category | Change Date Category Enum | DEPRECATED! Usechange_date_formatinstead |
| change_date_format | Reference ID forDate Format | The format of the change date |
| changed_company_id | Reference ID forCompany | The new ID for a company that has gone through a merger or restructuring |
| checksum | uuid | Hash of the object |
| country | Integer | ISO 3166-1 country code |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | A free text description of a company |
| developed | Array ofGameIDs | An array of games that a company has developed |
| logo | Reference ID forCompany Logo | The company’s logo |
| name | String | |
| parent | Reference ID forCompany | A company with a controlling interest in a specific company |
| published | Array ofGameIDs | An array of games that a company has published |
| slug | String | A url-safe, unique, lower-case version of the name |
| start_date | Unix Time Stamp | The date a company was founded |
| start_date_category | Start Date Category Enum | DEPRECATED! Usestart_date_formatinstead |
| start_date_format | Reference ID forDate Format | The format of the start date |
| status | Reference ID forCompany Status | The status of the company |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |
| websites | Array ofCompany WebsiteIDs | The companies official websites |


### Company Enums

change_date_category


| name | value |
| --- | --- |
| YYYYMMMMDD | 0 |
| YYYYMMMM | 1 |
| YYYY | 2 |
| YYYYQ1 | 3 |
| YYYYQ2 | 4 |
| YYYYQ3 | 5 |
| YYYYQ4 | 6 |
| TBD | 7 |

start_date_category


| name | value |
| --- | --- |
| YYYYMMMMDD | 0 |
| YYYYMMMM | 1 |
| YYYY | 2 |
| YYYYQ1 | 3 |
| YYYYQ2 | 4 |
| YYYYQ3 | 5 |
| YYYYQ4 | 6 |
| TBD | 7 |


## Company Logo


```python
from requests import post
response = post('https://api.igdb.com/v4/company_logos', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alpha_channel,animated,checksum,height,image_id,url,width;'})
print ("response: %s" % str(response.json()))
```

The logos of developers and publishers


### Request Path

`https://api.igdb.com/v4/company_logos`


| field | type | description |
| --- | --- | --- |
| alpha_channel | boolean | |
| animated | boolean | |
| checksum | uuid | Hash of the object |
| height | Integer | The height of the image in pixels |
| image_id | String | The ID of the image used to construct an IGDB image link |
| url | String | The website address (URL) of the item |
| width | Integer | The width of the image in pixels |


## Company Status


```python
from requests import post
response = post('https://api.igdb.com/v4/company_statuses', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Company Status


### Request Path

`https://api.igdb.com/v4/company_statuses`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Company Website


```python
from requests import post
response = post('https://api.igdb.com/v4/company_websites', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,trusted,type,url;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:

category: DEPRECATED! Use type instead

Company Website


### Request Path

`https://api.igdb.com/v4/company_websites`


| field | type | description |
| --- | --- | --- |
| category | Category Enum | DEPRECATED! Usetypeinstead |
| checksum | uuid | Hash of the object |
| trusted | boolean | |
| type | Reference ID forWebsite Type | The website type associated with the website |
| url | String | The website address (URL) of the item |


### Company Website Enums

category


| name | value |
| --- | --- |
| official | 1 |
| wikia | 2 |
| wikipedia | 3 |
| facebook | 4 |
| twitter | 5 |
| twitch | 6 |
| instagram | 8 |
| youtube | 9 |
| iphone | 10 |
| ipad | 11 |
| android | 12 |
| steam | 13 |
| reddit | 14 |
| itch | 15 |
| epicgames | 16 |
| gog | 17 |
| discord | 18 |
| bluesky | 19 |


## Cover


```python
from requests import post
response = post('https://api.igdb.com/v4/covers', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alpha_channel,animated,checksum,game,game_localization,height,image_id,url,width;'})
print ("response: %s" % str(response.json()))
```

The cover art of games


### Request Path

`https://api.igdb.com/v4/covers`


| field | type | description |
| --- | --- | --- |
| alpha_channel | boolean | |
| animated | boolean | |
| checksum | uuid | Hash of the object |
| game | Reference ID forGame | The game this cover is associated with. If it is empty then this 
cover belongs to a game_localization, which can be found under 
game_localization field |
| game_localization | Reference ID forGame Localization | The game localization this cover might be associated with |
| height | Integer | The height of the image in pixels |
| image_id | String | The ID of the image used to construct an IGDB image link |
| url | String | The website address (URL) of the item |
| width | Integer | The width of the image in pixels |


## Date Format


```python
from requests import post
response = post('https://api.igdb.com/v4/date_formats', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,format,updated_at;'})
print ("response: %s" % str(response.json()))
```

The Date Format


### Request Path

`https://api.igdb.com/v4/date_formats`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| format | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Event


```python
from requests import post
response = post('https://api.igdb.com/v4/events', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,description,end_time,event_logo,event_networks,games,live_stream_url,name,slug,start_time,time_zone,updated_at,videos;'})
print ("response: %s" % str(response.json()))
```

Gaming event like GamesCom, Tokyo Game Show, PAX or GSL


### Request Path

`https://api.igdb.com/v4/events`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | The description of the event |
| end_time | datetime | End time of the event in UTC |
| event_logo | Reference ID forEvent Logo | Logo of the event. |
| event_networks | Array ofEvent NetworkIDs | Urls associated with the event |
| games | Array ofGameIDs | Games featured in the event |
| live_stream_url | String | URL to the livestream of the event. |
| name | String | The name of the event |
| slug | String | A url-safe, unique, lower-case version of the name |
| start_time | datetime | Start time of the event in UTC |
| time_zone | String | Timezone the event is in. |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| videos | Array ofGame VideoIDs | Trailers featured in the event |


## Event Logo


```python
from requests import post
response = post('https://api.igdb.com/v4/event_logos', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alpha_channel,animated,checksum,created_at,event,height,image_id,updated_at,url,width;'})
print ("response: %s" % str(response.json()))
```

Logo for the event


### Request Path

`https://api.igdb.com/v4/event_logos`


| field | type | description |
| --- | --- | --- |
| alpha_channel | boolean | |
| animated | boolean | |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| event | Reference ID forEvent | The event associated with this logo. |
| height | Integer | The height of the image in pixels |
| image_id | String | The ID of the image used to construct an IGDB image link |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |
| width | Integer | The width of the image in pixels |


## Event Network


```python
from requests import post
response = post('https://api.igdb.com/v4/event_networks', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,event,network_type,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Urls related to the event like twitter, facebook and youtube


### Request Path

`https://api.igdb.com/v4/event_networks`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| event | Reference ID forEvent | The event associated with this URL. |
| network_type | Reference ID forNetwork Type | Network type |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## External Game


```python
from requests import post
response = post('https://api.igdb.com/v4/external_games', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,countries,created_at,external_game_source,game,game_release_format,media,name,platform,uid,updated_at,url,year;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:


category: DEPRECATED! Use external_game_source instead


media: DEPRECATED! Use game_release_format instead

Game IDs on other services


### Request Path

`https://api.igdb.com/v4/external_games`


| field | type | description |
| --- | --- | --- |
| category | Category Enum | DEPRECATED! Useexternal_game_sourceinstead |
| checksum | uuid | Hash of the object |
| countries | Array of Integers | The ISO country code of the external game product. |
| created_at | datetime | Date this was initially added to the IGDB database |
| external_game_source | Reference ID forExternal Game Source | The source of the external game. |
| game | Reference ID forGame | The IGDB ID of the game |
| game_release_format | Reference ID forGame Release Format | The release format of the external game. |
| media | Media Enum | DEPRECATED! Usegame_release_formatinstead |
| name | String | The name of the game according to the other service |
| platform | Reference ID forPlatform | The platform of the external game product. |
| uid | String | The other services ID for this game |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |
| year | Integer | The year in full (2018) |


### External Game Enums

category


| name | value |
| --- | --- |
| steam | 1 |
| gog | 5 |
| youtube | 10 |
| microsoft | 11 |
| apple | 13 |
| twitch | 14 |
| android | 15 |
| amazon_asin | 20 |
| amazon_luna | 22 |
| amazon_adg | 23 |
| epic_game_store | 26 |
| oculus | 28 |
| utomik | 29 |
| itch_io | 30 |
| xbox_marketplace | 31 |
| kartridge | 32 |
| playstation_store_us | 36 |
| focus_entertainment | 37 |
| xbox_game_pass_ultimate_cloud | 54 |
| gamejolt | 55 |

media


| name | value |
| --- | --- |
| digital | 1 |
| physical | 2 |


## External Game Source


```python
from requests import post
response = post('https://api.igdb.com/v4/external_game_sources', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Sources for the external games


### Request Path

`https://api.igdb.com/v4/external_game_sources`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Franchise


```python
from requests import post
response = post('https://api.igdb.com/v4/franchises', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,games,name,slug,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

A list of video game franchises such as Star Wars.


### Request Path

`https://api.igdb.com/v4/franchises`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| games | Array ofGameIDs | The games that are associated with this franchise |
| name | String | The name of the franchise |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Game


```python
from requests import post
response = post('https://api.igdb.com/v4/games', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields age_ratings,aggregated_rating,aggregated_rating_count,alternative_names,artworks,bundles,category,checksum,collection,collections,cover,created_at,dlcs,expanded_games,expansions,external_games,first_release_date,follows,forks,franchise,franchises,game_engines,game_localizations,game_modes,game_status,game_type,genres,hypes,involved_companies,keywords,language_supports,multiplayer_modes,name,parent_game,platforms,player_perspectives,ports,rating,rating_count,release_dates,remakes,remasters,screenshots,similar_games,slug,standalone_expansions,status,storyline,summary,tags,themes,total_rating,total_rating_count,updated_at,url,version_parent,version_title,videos,websites;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:


category: DEPRECATED! Use game_type instead


collection: DEPRECATED! Use collections instead


follows: DEPRECATED! - To be removed


status: DEPRECATED! Use game_status instead

Video Games!


### Request Path

`https://api.igdb.com/v4/games`


| field | type | description |
| --- | --- | --- |
| age_ratings | Array ofAge RatingIDs | The PEGI rating |
| aggregated_rating | Double | Rating based on external critic scores |
| aggregated_rating_count | Integer | Number of external critic scores |
| alternative_names | Array ofAlternative NameIDs | Alternative names for this game |
| artworks | Array ofArtworkIDs | Artworks of this game |
| bundles | Array ofGameIDs | The bundles this game is a part of |
| category | Category Enum | DEPRECATED! Usegame_typeinstead |
| checksum | uuid | Hash of the object |
| collection | Reference ID forCollection | DEPRECATED! Usecollectionsinstead |
| collections | Array ofCollectionIDs | The collections that this game is in. |
| cover | Reference ID forCover | The cover of this game |
| created_at | datetime | Date this was initially added to the IGDB database |
| dlcs | Array ofGameIDs | DLCs for this game |
| expanded_games | Array ofGameIDs | Expanded games of this game |
| expansions | Array ofGameIDs | Expansions of this game |
| external_games | Array ofExternal GameIDs | External IDs this game has on other services |
| first_release_date | Unix Time Stamp | The first release date for this game |
| follows | Integer | DEPRECATED! - To be removed |
| forks | Array ofGameIDs | Forks of this game |
| franchise | Reference ID forFranchise | The main franchise |
| franchises | Array ofFranchiseIDs | Other franchises the game belongs to |
| game_engines | Array ofGame EngineIDs | The game engine used in this game |
| game_localizations | Array ofGame LocalizationIDs | Supported game localizations for this game. A region can have at most one game localization for a given game |
| game_modes | Array ofGame ModeIDs | Modes of gameplay |
| game_status | Reference ID forGame Status | The status of the games release |
| game_type | Reference ID forGame Type | The type of game |
| genres | Array ofGenreIDs | Genres of the game |
| hypes | Integer | Number of follows a game gets before release |
| involved_companies | Array ofInvolved CompanyIDs | Companies who developed this game |
| keywords | Array ofKeywordIDs | Associated keywords |
| language_supports | Array ofLanguage SupportIDs | Supported Languages for this game |
| multiplayer_modes | Array ofMultiplayer ModeIDs | Multiplayer modes for this game |
| name | String | |
| parent_game | Reference ID forGame | If a DLC, expansion or part of a bundle, this is the main game or bundle |
| platforms | Array ofPlatformIDs | Platforms this game was released on |
| player_perspectives | Array ofPlayer PerspectiveIDs | The main perspective of the player |
| ports | Array ofGameIDs | Ports of this game |
| rating | Double | Average IGDB user rating |
| rating_count | Integer | Total number of IGDB user ratings |
| release_dates | Array ofRelease DateIDs | Release dates of this game |
| remakes | Array ofGameIDs | Remakes of this game |
| remasters | Array ofGameIDs | Remasters of this game |
| screenshots | Array ofScreenshotIDs | Screenshots of this game |
| similar_games | Array ofGameIDs | Similar games |
| slug | String | A url-safe, unique, lower-case version of the name |
| standalone_expansions | Array ofGameIDs | Standalone expansions of this game |
| status | Status Enum | DEPRECATED! Usegame_statusinstead |
| storyline | String | A short description of a games story |
| summary | String | A description of the game |
| tags | Array ofTag Numbers | Related entities in the IGDB API |
| themes | Array ofThemeIDs | Themes of the game |
| total_rating | Double | Average rating based on both IGDB user and external critic scores |
| total_rating_count | Integer | Total number of user and external critic scores |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |
| version_parent | Reference ID forGame | If a version, this is the main game |
| version_title | String | Title of this version (i.e Gold edition) |
| videos | Array ofGame VideoIDs | Videos of this game |
| websites | Array ofWebsiteIDs | Websites associated with this game |


### Game Enums

category


| name | value |
| --- | --- |
| main_game | 0 |
| dlc_addon | 1 |
| expansion | 2 |
| bundle | 3 |
| standalone_expansion | 4 |
| mod | 5 |
| episode | 6 |
| season | 7 |
| remake | 8 |
| remaster | 9 |
| expanded_game | 10 |
| port | 11 |
| fork | 12 |
| pack | 13 |
| update | 14 |

status


| name | value |
| --- | --- |
| released | 0 |
| alpha | 2 |
| beta | 3 |
| early_access | 4 |
| offline | 5 |
| cancelled | 6 |
| rumored | 7 |
| delisted | 8 |


## Game Engine


```python
from requests import post
response = post('https://api.igdb.com/v4/game_engines', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,companies,created_at,description,logo,name,platforms,slug,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Video game engines such as unreal engine.


### Request Path

`https://api.igdb.com/v4/game_engines`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| companies | Array ofCompanyIDs | Companies who used this game engine |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | Description of the game engine |
| logo | Reference ID forGame Engine Logo | Logo of the game engine |
| name | String | Name of the game engine |
| platforms | Array ofPlatformIDs | Platforms this game engine was deployed on |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Game Engine Logo


```python
from requests import post
response = post('https://api.igdb.com/v4/game_engine_logos', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alpha_channel,animated,checksum,height,image_id,url,width;'})
print ("response: %s" % str(response.json()))
```

The logos of game engines


### Request Path

`https://api.igdb.com/v4/game_engine_logos`


| field | type | description |
| --- | --- | --- |
| alpha_channel | boolean | |
| animated | boolean | |
| checksum | uuid | Hash of the object |
| height | Integer | The height of the image in pixels |
| image_id | String | The ID of the image used to construct an IGDB image link |
| url | String | The website address (URL) of the item |
| width | Integer | The width of the image in pixels |


## Game Localization


```python
from requests import post
response = post('https://api.igdb.com/v4/game_localizations', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,cover,created_at,game,name,region,updated_at;'})
print ("response: %s" % str(response.json()))
```

Game localization for a game


### Request Path

`https://api.igdb.com/v4/game_localizations`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| cover | Reference ID forCover | The cover of this game localization |
| created_at | datetime | Date this was initially added to the IGDB database |
| game | Reference ID forGame | The Game the localization belongs to |
| name | String | |
| region | Reference ID forRegion | The Region of the localization |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Game Mode


```python
from requests import post
response = post('https://api.igdb.com/v4/game_modes', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,slug,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Single player, Multiplayer etc


### Request Path

`https://api.igdb.com/v4/game_modes`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | The name of the game mode |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Game Release Format


```python
from requests import post
response = post('https://api.igdb.com/v4/game_release_formats', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,format,updated_at;'})
print ("response: %s" % str(response.json()))
```

The format of the game release


### Request Path

`https://api.igdb.com/v4/game_release_formats`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| format | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Game Status


```python
from requests import post
response = post('https://api.igdb.com/v4/game_statuses', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,status,updated_at;'})
print ("response: %s" % str(response.json()))
```

The release status of the game


### Request Path

`https://api.igdb.com/v4/game_statuses`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| status | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Game Time To Beat


```python
from requests import post
response = post('https://api.igdb.com/v4/game_time_to_beats', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,completely,count,created_at,game_id,hastily,normally,updated_at;'})
print ("response: %s" % str(response.json()))
```

Average time to beat times for a game.


### Request Path

`https://api.igdb.com/v4/game_time_to_beats`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| completely | Integer | Average time (in seconds) to finish the game to 100% completion. |
| count | Integer | Total number of time to beat submissions for this game |
| created_at | datetime | Date this was initially added to the IGDB database |
| game_id | Integer | The ID of the game associated with the time to beat data |
| hastily | Integer | Average time (in seconds) to finish the game to its credits without spending notable time on extras such as side quests. |
| normally | Integer | Average time (in seconds) to finish the game while mixing in some extras such as side quests without being overly thorough. |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Game Type


```python
from requests import post
response = post('https://api.igdb.com/v4/game_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,type,updated_at;'})
print ("response: %s" % str(response.json()))
```

The type that this game is


### Request Path

`https://api.igdb.com/v4/game_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| type | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Game Version


```python
from requests import post
response = post('https://api.igdb.com/v4/game_versions', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,features,game,games,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Details about game editions and versions.


### Request Path

`https://api.igdb.com/v4/game_versions`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| features | Array ofGame Version FeatureIDs | Features and descriptions of what makes each version/edition different from the main game |
| game | Reference ID forGame | The game these versions/editions are of |
| games | Array ofGameIDs | Game Versions and Editions |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Game Version Feature


```python
from requests import post
response = post('https://api.igdb.com/v4/game_version_features', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,description,position,title,values;'})
print ("response: %s" % str(response.json()))
```

Features and descriptions of what makes each version/edition different from the main game


### Request Path

`https://api.igdb.com/v4/game_version_features`


| field | type | description |
| --- | --- | --- |
| category | Category Enum | The category of the feature description |
| checksum | uuid | Hash of the object |
| description | String | The description of the feature |
| position | Integer | Position of this feature in the list of features |
| title | String | The title of the feature |
| values | Array ofGame Version Feature ValueIDs | The bool/text value of the feature |


### Game Version Feature Enums

category


| name | value |
| --- | --- |
| boolean | 0 |
| description | 1 |


## Game Version Feature Value


```python
from requests import post
response = post('https://api.igdb.com/v4/game_version_feature_values', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,game,game_feature,included_feature,note;'})
print ("response: %s" % str(response.json()))
```

The bool/text value of the feature


### Request Path

`https://api.igdb.com/v4/game_version_feature_values`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| game | Reference ID forGame | The version/edition this value refers to |
| game_feature | Reference ID forGame Version Feature | The id of the game feature |
| included_feature | Included Feature Enum | The boole value of this feature |
| note | String | The text value of this feature |


### Game Version Feature Value Enums

included_feature


| name | value |
| --- | --- |
| NOT_INCLUDED | 0 |
| INCLUDED | 1 |
| PRE_ORDER_ONLY | 2 |


## Game Video


```python
from requests import post
response = post('https://api.igdb.com/v4/game_videos', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,game,name,video_id;'})
print ("response: %s" % str(response.json()))
```

A video associated with a game


### Request Path

`https://api.igdb.com/v4/game_videos`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| game | Reference ID forGame | The game this video is associated with |
| name | String | The name of the video |
| video_id | String | The external ID of the video (YouTube Links) |


## Genre


```python
from requests import post
response = post('https://api.igdb.com/v4/genres', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,slug,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Genres of video game


### Request Path

`https://api.igdb.com/v4/genres`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Involved Company


```python
from requests import post
response = post('https://api.igdb.com/v4/involved_companies', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,company,created_at,developer,game,porting,publisher,supporting,updated_at;'})
print ("response: %s" % str(response.json()))
```


### Request Path

`https://api.igdb.com/v4/involved_companies`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| company | Reference ID forCompany | |
| created_at | datetime | Date this was initially added to the IGDB database |
| developer | boolean | |
| game | Reference ID forGame | |
| porting | boolean | |
| publisher | boolean | |
| supporting | boolean | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Keyword


```python
from requests import post
response = post('https://api.igdb.com/v4/keywords', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,slug,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Keywords are words or phrases that get tagged to a game such as “world war 2” or “steampunk”.


### Request Path

`https://api.igdb.com/v4/keywords`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Language


```python
from requests import post
response = post('https://api.igdb.com/v4/languages', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,locale,name,native_name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Languages that are used in the Language Support endpoint.


### Request Path

`https://api.igdb.com/v4/languages`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| locale | String | The combination of Language code and Country code |
| name | String | The English name of the Language |
| native_name | String | The Native Name of the Language |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Language Support


```python
from requests import post
response = post('https://api.igdb.com/v4/language_supports', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,game,language,language_support_type,updated_at;'})
print ("response: %s" % str(response.json()))
```

Games can be played with different languages for voice acting, subtitles, or the interface language.


### Request Path

`https://api.igdb.com/v4/language_supports`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| game | Reference ID forGame | |
| language | Reference ID forLanguage | |
| language_support_type | Reference ID forLanguage Support Type | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Language Support Type


```python
from requests import post
response = post('https://api.igdb.com/v4/language_support_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Language Support Types contains the identifiers for the support types that Language Support uses.


### Request Path

`https://api.igdb.com/v4/language_support_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Multiplayer Mode


```python
from requests import post
response = post('https://api.igdb.com/v4/multiplayer_modes', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields campaigncoop,checksum,dropin,game,lancoop,offlinecoop,offlinecoopmax,offlinemax,onlinecoop,onlinecoopmax,onlinemax,platform,splitscreen,splitscreenonline;'})
print ("response: %s" % str(response.json()))
```

Data about the supported multiplayer types


### Request Path

`https://api.igdb.com/v4/multiplayer_modes`


| field | type | description |
| --- | --- | --- |
| campaigncoop | boolean | True if the game supports campaign coop |
| checksum | uuid | Hash of the object |
| dropin | boolean | True if the game supports drop in/out multiplayer |
| game | Reference ID forGame | The game this multiplayer mode is associated with |
| lancoop | boolean | True if the game supports LAN coop |
| offlinecoop | boolean | True if the game supports offline coop |
| offlinecoopmax | Integer | Maximum number of offline players in offline coop |
| offlinemax | Integer | Maximum number of players in offline multiplayer |
| onlinecoop | boolean | True if the game supports online coop |
| onlinecoopmax | Integer | Maximum number of online players in online coop |
| onlinemax | Integer | Maximum number of players in online multiplayer |
| platform | Reference ID forPlatform | The platform this multiplayer mode refers to |
| splitscreen | boolean | True if the game supports split screen, offline multiplayer |
| splitscreenonline | boolean | True if the game supports split screen, online multiplayer |


## Network Type


```python
from requests import post
response = post('https://api.igdb.com/v4/network_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,event_networks,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Social networks related to the event like twitter, facebook and youtube


### Request Path

`https://api.igdb.com/v4/network_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| event_networks | Array ofEvent NetworkIDs | Urls associated with the event type |
| name | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Platform


```python
from requests import post
response = post('https://api.igdb.com/v4/platforms', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields abbreviation,alternative_name,category,checksum,created_at,generation,name,platform_family,platform_logo,platform_type,slug,summary,updated_at,url,versions,websites;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:

category: DEPRECATED! Use platform_type instead

The hardware used to run the game or game delivery network


### Request Path

`https://api.igdb.com/v4/platforms`


| field | type | description |
| --- | --- | --- |
| abbreviation | String | An abbreviation of the platform name |
| alternative_name | String | An alternative name for the platform |
| category | Category Enum | DEPRECATED! Useplatform_typeinstead |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| generation | Integer | The generation of the platform |
| name | String | The name of the platform |
| platform_family | Reference ID forPlatform Family | The family of platforms this one belongs to |
| platform_logo | Reference ID forPlatform Logo | The logo of the first Version of this platform |
| platform_type | Reference ID forPlatform Type | The type of the platform |
| slug | String | A url-safe, unique, lower-case version of the name |
| summary | String | The summary of the first Version of this platform |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |
| versions | Array ofPlatform VersionIDs | Associated versions of this platform |
| websites | Array ofPlatform WebsiteIDs | The main website |


### Platform Enums

category


| name | value |
| --- | --- |
| console | 1 |
| arcade | 2 |
| platform | 3 |
| operating_system | 4 |
| portable_console | 5 |
| computer | 6 |


## Platform Family


```python
from requests import post
response = post('https://api.igdb.com/v4/platform_families', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,name,slug;'})
print ("response: %s" % str(response.json()))
```

A collection of closely related platforms


### Request Path

`https://api.igdb.com/v4/platform_families`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| name | String | The name of the platform family |
| slug | String | A url-safe, unique, lower-case version of the name |


## Platform Logo


```python
from requests import post
response = post('https://api.igdb.com/v4/platform_logos', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alpha_channel,animated,checksum,height,image_id,url,width;'})
print ("response: %s" % str(response.json()))
```

Logo for a platform


### Request Path

`https://api.igdb.com/v4/platform_logos`


| field | type | description |
| --- | --- | --- |
| alpha_channel | boolean | |
| animated | boolean | |
| checksum | uuid | Hash of the object |
| height | Integer | The height of the image in pixels |
| image_id | String | The ID of the image used to construct an IGDB image link |
| url | String | The website address (URL) of the item |
| width | Integer | The width of the image in pixels |


## Platform Type


```python
from requests import post
response = post('https://api.igdb.com/v4/platform_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Types of platforms


### Request Path

`https://api.igdb.com/v4/platform_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Platform Version


```python
from requests import post
response = post('https://api.igdb.com/v4/platform_versions', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,companies,connectivity,cpu,graphics,main_manufacturer,media,memory,name,os,output,platform_logo,platform_version_release_dates,resolutions,slug,sound,storage,summary,url;'})
print ("response: %s" % str(response.json()))
```


### Request Path

`https://api.igdb.com/v4/platform_versions`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| companies | Array ofPlatform Version CompanyIDs | Who developed this platform version |
| connectivity | String | The network capabilities |
| cpu | String | The integrated control processing unit |
| graphics | String | The graphics chipset |
| main_manufacturer | Reference ID forPlatform Version Company | Who manufactured this version of the platform |
| media | String | The type of media this version accepted |
| memory | String | How much memory there is |
| name | String | The name of the platform version |
| os | String | The operating system installed on the platform version |
| output | String | The output video rate |
| platform_logo | Reference ID forPlatform Logo | The logo of this platform version |
| platform_version_release_dates | Array ofPlatform Version Release DateIDs | When this platform was released |
| resolutions | String | The maximum resolution |
| slug | String | A url-safe, unique, lower-case version of the name |
| sound | String | The sound chipset |
| storage | String | How much storage there is |
| summary | String | A short summary |
| url | String | The website address (URL) of the item |


## Platform Version Company


```python
from requests import post
response = post('https://api.igdb.com/v4/platform_version_companies', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,comment,company,developer,manufacturer;'})
print ("response: %s" % str(response.json()))
```

A platform developer


### Request Path

`https://api.igdb.com/v4/platform_version_companies`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| comment | String | Any notable comments about the developer |
| company | Reference ID forCompany | The company responsible for developing this platform version |
| developer | boolean | |
| manufacturer | boolean | |


## Platform Version Release Date


```python
from requests import post
response = post('https://api.igdb.com/v4/platform_version_release_dates', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,created_at,date,date_format,human,m,platform_version,region,release_region,updated_at,y;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:


category: DEPRECATED! Use date_format instead


region: DEPRECATED! Use release_region instead

A handy endpoint that extends platform release dates. Used to dig deeper into release dates, platforms and versions.


### Request Path

`https://api.igdb.com/v4/platform_version_release_dates`


| field | type | description |
| --- | --- | --- |
| category | Category Enum | DEPRECATED! Usedate_formatinstead |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| date | Unix Time Stamp | The release date |
| date_format | Reference ID forDate Format | The format of the change date |
| human | String | A human readable version of the release date |
| m | Integer | The month as an integer starting at 1 (January) |
| platform_version | Reference ID forPlatform Version | The platform this release date is for |
| region | Region Enum | DEPRECATED! Userelease_regioninstead |
| release_region | Reference ID forRelease Date Region | The region of the release |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| y | Integer | The year in full (2018) |


### Platform Version Release Date Enums

category


| name | value |
| --- | --- |
| YYYYMMMMDD | 0 |
| YYYYMMMM | 1 |
| YYYY | 2 |
| YYYYQ1 | 3 |
| YYYYQ2 | 4 |
| YYYYQ3 | 5 |
| YYYYQ4 | 6 |
| TBD | 7 |

region


| name | value |
| --- | --- |
| europe | 1 |
| north_america | 2 |
| australia | 3 |
| new_zealand | 4 |
| japan | 5 |
| china | 6 |
| asia | 7 |
| worldwide | 8 |
| korea | 9 |
| brazil | 10 |


## Platform Website


```python
from requests import post
response = post('https://api.igdb.com/v4/platform_websites', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,trusted,type,url;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:

category: DEPRECATED! Use type instead

The main website for the platform


### Request Path

`https://api.igdb.com/v4/platform_websites`


| field | type | description |
| --- | --- | --- |
| category | Category Enum | DEPRECATED! Usetypeinstead |
| checksum | uuid | Hash of the object |
| trusted | boolean | |
| type | Reference ID forWebsite Type | The website type associated with the website |
| url | String | The website address (URL) of the item |


### Platform Website Enums

category


| name | value |
| --- | --- |
| official | 1 |
| wikia | 2 |
| wikipedia | 3 |
| facebook | 4 |
| twitter | 5 |
| twitch | 6 |
| instagram | 8 |
| youtube | 9 |
| iphone | 10 |
| ipad | 11 |
| android | 12 |
| steam | 13 |
| reddit | 14 |
| discord | 15 |
| google_plus | 16 |
| tumblr | 17 |
| linkedin | 18 |
| pinterest | 19 |
| soundcloud | 20 |


## Player Perspective


```python
from requests import post
response = post('https://api.igdb.com/v4/player_perspectives', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,slug,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Player perspectives describe the view/perspective of the player in a video game.


### Request Path

`https://api.igdb.com/v4/player_perspectives`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Popularity Primitive


```python
from requests import post
response = post('https://api.igdb.com/v4/popularity_primitives', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields calculated_at,checksum,created_at,external_popularity_source,game_id,popularity_source,popularity_type,updated_at,value;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:

popularity_source: DEPRECATED! Use external_popularity_source instead

Popularity Primitives, this endpoint lists available primitives with their source and popularity type.


### Request Path

`https://api.igdb.com/v4/popularity_primitives`


| field | type | description |
| --- | --- | --- |
| calculated_at | datetime | |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| external_popularity_source | Reference ID forExternal Game Source | |
| game_id | Integer | |
| popularity_source | Popularity Source Enum | DEPRECATED! Useexternal_popularity_sourceinstead |
| popularity_type | Reference ID forPopularity Type | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| value | bigdecimal | |


### Popularity Primitive Enums

popularity_source


| name | value |
| --- | --- |
| igdb | 121 |


## Popularity Type


```python
from requests import post
response = post('https://api.igdb.com/v4/popularity_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,external_popularity_source,name,popularity_source,updated_at;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:

popularity_source: DEPRECATED! Use external_popularity_source instead

This describes what type of popularity primitive or popularity indicator the popularity value is.


### Request Path

`https://api.igdb.com/v4/popularity_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| external_popularity_source | Reference ID forExternal Game Source | |
| name | String | |
| popularity_source | Popularity Source Enum | DEPRECATED! Useexternal_popularity_sourceinstead |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


### Popularity Type Enums

popularity_source


| name | value |
| --- | --- |
| steam | 1 |
| igdb | 121 |


## Region


```python
from requests import post
response = post('https://api.igdb.com/v4/regions', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,created_at,identifier,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

Region for game localization


### Request Path

`https://api.igdb.com/v4/regions`


| field | type | description |
| --- | --- | --- |
| category | String | This can be either ’locale’ or ‘continent’ |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| identifier | String | This is the identifier of each region |
| name | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Release Date


```python
from requests import post
response = post('https://api.igdb.com/v4/release_dates', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,created_at,date,date_format,game,human,m,platform,region,release_region,status,updated_at,y;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:


category: DEPRECATED! Use date_format instead


region: DEPRECATED! Use release_region instead

A handy endpoint that extends game release dates. Used to dig deeper into release dates, platforms and versions.


### Request Path

`https://api.igdb.com/v4/release_dates`


| field | type | description |
| --- | --- | --- |
| category | Category Enum | DEPRECATED! Usedate_formatinstead |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| date | datetime | The date of the release |
| date_format | Reference ID forDate Format | The format of the change date |
| game | Reference ID forGame | |
| human | String | A human readable representation of the date |
| m | Integer | The month as an integer starting at 1 (January) |
| platform | Reference ID forPlatform | The platform of the release |
| region | Region Enum | DEPRECATED! Userelease_regioninstead |
| release_region | Reference ID forRelease Date Region | The region of the release |
| status | Reference ID forRelease Date Status | The status of the release. |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| y | Integer | The year in full (2018) |


### Release Date Enums

category


| name | value |
| --- | --- |
| YYYYMMMMDD | 0 |
| YYYYMMMM | 1 |
| YYYY | 2 |
| YYYYQ1 | 3 |
| YYYYQ2 | 4 |
| YYYYQ3 | 5 |
| YYYYQ4 | 6 |
| TBD | 7 |

region


| name | value |
| --- | --- |
| europe | 1 |
| north_america | 2 |
| australia | 3 |
| new_zealand | 4 |
| japan | 5 |
| china | 6 |
| asia | 7 |
| worldwide | 8 |
| korea | 9 |
| brazil | 10 |


## Release Date Region


```python
from requests import post
response = post('https://api.igdb.com/v4/release_date_regions', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,region,updated_at;'})
print ("response: %s" % str(response.json()))
```

Regions for release dates


### Request Path

`https://api.igdb.com/v4/release_date_regions`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| region | String | |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Release Date Status


```python
from requests import post
response = post('https://api.igdb.com/v4/release_date_statuses', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,description,name,updated_at;'})
print ("response: %s" % str(response.json()))
```

An endpoint to provide definition of all of the current release date statuses.


### Request Path

`https://api.igdb.com/v4/release_date_statuses`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| description | String | The description of the release date status. |
| name | String | The name of the release date status. |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


## Screenshot


```python
from requests import post
response = post('https://api.igdb.com/v4/screenshots', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alpha_channel,animated,checksum,game,height,image_id,url,width;'})
print ("response: %s" % str(response.json()))
```

Screenshots of games


### Request Path

`https://api.igdb.com/v4/screenshots`


| field | type | description |
| --- | --- | --- |
| alpha_channel | boolean | |
| animated | boolean | |
| checksum | uuid | Hash of the object |
| game | Reference ID forGame | The game this video is associated with |
| height | Integer | The height of the image in pixels |
| image_id | String | The ID of the image used to construct an IGDB image link |
| url | String | The website address (URL) of the item |
| width | Integer | The width of the image in pixels |


## Search


```python
from requests import post
response = post('https://api.igdb.com/v4/search', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields alternative_name,character,checksum,collection,company,description,game,name,platform,published_at,test_dummy,theme;'})
print ("response: %s" % str(response.json()))
```


### Request Path

`https://api.igdb.com/v4/search`


| field | type | description |
| --- | --- | --- |
| alternative_name | String | |
| character | Reference ID forCharacter | |
| checksum | uuid | Hash of the object |
| collection | Reference ID forCollection | |
| company | Reference ID forCompany | |
| description | String | |
| game | Reference ID forGame | |
| name | String | |
| platform | Reference ID forPlatform | |
| published_at | Unix Time Stamp | The date this item was initially published by the third party |
| test_dummy | Reference ID forTest Dummy | |
| theme | Reference ID forTheme | |


## Theme


```python
from requests import post
response = post('https://api.igdb.com/v4/themes', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,name,slug,updated_at,url;'})
print ("response: %s" % str(response.json()))
```

Video game themes


### Request Path

`https://api.igdb.com/v4/themes`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| name | String | |
| slug | String | A url-safe, unique, lower-case version of the name |
| updated_at | datetime | The last date this entry was updated in the IGDB database |
| url | String | The website address (URL) of the item |


## Website


```python
from requests import post
response = post('https://api.igdb.com/v4/websites', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields category,checksum,game,trusted,type,url;'})
print ("response: %s" % str(response.json()))
```

> **Warning:** Deprecated Fields:

category: DEPRECATED! Use type instead

A website url, usually associated with a game


### Request Path

`https://api.igdb.com/v4/websites`


| field | type | description |
| --- | --- | --- |
| category | Category Enum | DEPRECATED! Usetypeinstead |
| checksum | uuid | Hash of the object |
| game | Reference ID forGame | The game this website is associated with |
| trusted | boolean | |
| type | Reference ID forWebsite Type | The website type associated with the website |
| url | String | The website address (URL) of the item |


### Website Enums

category


| name | value |
| --- | --- |
| official | 1 |
| wikia | 2 |
| wikipedia | 3 |
| facebook | 4 |
| twitter | 5 |
| twitch | 6 |
| instagram | 8 |
| youtube | 9 |
| iphone | 10 |
| ipad | 11 |
| android | 12 |
| steam | 13 |
| reddit | 14 |
| itch | 15 |
| epicgames | 16 |
| gog | 17 |
| discord | 18 |
| bluesky | 19 |


## Website Type


```python
from requests import post
response = post('https://api.igdb.com/v4/website_types', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'},'data': 'fields checksum,created_at,type,updated_at;'})
print ("response: %s" % str(response.json()))
```

A website type, usually the name of the website


### Request Path

`https://api.igdb.com/v4/website_types`


| field | type | description |
| --- | --- | --- |
| checksum | uuid | Hash of the object |
| created_at | datetime | Date this was initially added to the IGDB database |
| type | String | The website type |
| updated_at | datetime | The last date this entry was updated in the IGDB database |


# PopScore


## Introducing IGDB PopScore - Your key to tracking the latest trends in the video game market.

Accessible through our API, IGDB PopScore offers “popularity 
primitives” from sources like IGDB page visits and list additions, with 
more sources and primitives coming in the future.

With IGDB PopScore, you can define and create your own trend and 
popularity indicators using individual primitives or by combining them 
to fit your needs. Updated every 24 hours, our data ensures you always 
have the latest insights into the gaming market covering all platforms. 
Stay ahead of the curve, uncover emerging trends, and improve your 
data-driven decisions by checking out IGDB PopScore today


## Currently available PopScore primitives

1. IGDB Visits: Game page visits on IGDB.com.
2. “IGDB Want to Play”: Additions to IGDB.com users’ “Want to Play” lists.
3. “IGDB Playing”: Additions to IGDB.com users’ “Playing” lists.
4. “IGDB Played”: Additions to IGDB.com users’ “Played” lists.
5. “Steam 24hr Peak Players”: Peak CCU over the past 24 hours.
6. “Steam Postitive Reviews”: Total number of positive reviews
7. “Steam Negative Reviews”: Total number of negative reviews.
8. “Steam Total Reviews”: Total number of reviews (positive and negative).

We’re constantly refining and expanding IGDB PopScore to ensure you 
have access to the most up-to-date and relevant data as possible. Stay 
tuned for exciting new features and data points as we continue to push 
the boundaries of what’s possible in the realm of video game trend 
analysis.

You can check the current popularity types we support. to do so feel free to query the api /popularity-types.
More details can be found under [Popularity Types](#popularity-type)


## How to use Popularity API

Start by discovering the available popularity types mentioned above

Result:


```json
[
	{
		"id": 1,
		"popularity_source": 121,
		"name": "Visits",
	},
	{
		"id": 2,
		"popularity_source": 121,
		"name": "Want to Play",
	},
	{
		"id": 3,
		"popularity_source": 121,
		"name": "Playing",
	},
	{
		"id": 4,
		"popularity_source": 121,
		"name": "Played",
	},
	{
		"id": 5,
		"popularity_source": 1,
		"name": "24hr Peak Players",
	},
	{
		"id": 6,
		"popularity_source": 1,
		"name": "Postitive Reviews",
	},
	{
		"id": 7,
		"popularity_source": 1,
		"name": "Negative Reviews",
	},
	{
		"id": 8,
		"popularity_source": 1,
		"name": "Total Reviews",
	}
]
```

Below you can find some example use case of how you can use popularity primitives


### Use case 1:

As an API user I’d like to fetch the top 10 games based on IGDB visits.

Query:

Result:


```json
[
 {
 "id": 15456,
 "game_id": 121,
 "popularity_type": 1,
 "value": 0.006605335786569
 },
 {
 "id": 16211,
 "game_id": 1244,
 "popularity_type": 1,
 "value": 0.005482980680773
 },
...
 {
 "id": 33353,
 "game_id": 135400,
 "popularity_type": 1,
 "value": 0.002741490340386
 },
 {
 "id": 17317,
 "game_id": 3277,
 "popularity_type": 1,
 "value": 0.002695492180313
 }
]
```

In the preview of the result above you can see that the top game is the game with IGDB ID 121.
[https://www.igdb.com/games/minecraft](https://www.igdb.com/games/minecraft) # ID 121


### Use case 2:

Define your own trend metrics.
For example, by combining two or more primitives with equal or different
 weight ratios, you can create a unique popularity metric and ensure it 
fits your needs.

In our use case here we want to combine “Want to play” with a weight of 0.6 and “Playing” with a remaining weight of 0.4.

For each game we would need to calculate this in a matter of: 0.6 * 
(“Want to play” -> value) + 0.4 * (“Playing” -> value) = 
custom_popularity_value

To do that you would have to pull the values for each popularity type in your own system and then combine them accordingly.

To pull the date you’ll need locally a data structure similar to the 
existing popularity primitive where it will have the attributes.

game_id, popularity_type, value
The Want to Play maps to the popularity_type = 2 while Playing = 3

An example SQL query that would give you the top 100 of your custom_popularity would be

You can find detailed API documentation under [Popularity Types](#popularity-type) and [Popularity Primitives](#popularity-primitive)


# Webhooks


### What?

Webhooks allow us to push data to you when it is added, updated, or 
deleted. Instead of polling the API for changes, you can listen on your 
own HTTP endpoint (Webhook) and we will deliver the data to you.

Using Webhooks will ensure that your data is always up to date!


```python
from requests import post
response = post(
    'https://api.igdb.com/v4/ENDPOINT/webhooks/',
    headers={
        'Client-ID': 'YOUR_CLIENT_ID',
        'Authorization': 'Bearer YOUR_ACCESS_TOKEN',
        'Content-Type': 'application/x-www-form-urlencoded'
    },
    data='url=YOUR_WEBHOOK_URL&secret=YOUR_WEBHOOK_SECRET&method=create'
)
```


### How to register your webhook

To register a new webhook you need to send a `POST` request to `ENDPOINT/webhooks`. The endpoint is required as it specifies what type of data you want from your webhook.

The post request should contain `x-www-form-urlencoded` body with three parameters:

- `url` this is your prepared url that is ready to accept data from us.
- `method` this is the type of data you are expecting to your url, there are three types of methods

create, sends new items from the API
delete, sends deleted items from the API
update, sends updated items from the API
- `secret` this is your “secret” password for your webhook.
 Every request from the webhook service will have your secret in the 
header called `X-Secret`.


```yaml
// Example response upon registering your webhook
{ 
 "id": WEBHOOK_ID, // A unique ID for the webhook
 "url": "YOUR_WEBHOOK_URL", // Your chosen URL
 "category": 1, // Based on the endpoint you chose
 "sub_category": 0, // Based on your method (can be 0, 1, 2)
 "active": true, // Is the webhook currently active
 "api_key": "YOUR_CLIENT_ID", // Displays the api key the webhook is connected to
 "secret": "YOUR_SECRET", // Your chosen secret
 "created_at": "2018-11-25T23:00:00.000Z", // Created at date
 "updated_at": "2018-11-25T23:00:00.000Z" // Updated at date
}
```

Registering your webhook in Postman
Once your webhook is registered you will receive a response with the new webhook object


```yaml
// Delete Response from Webhook
{
 "id": "1234"
}
```

That’s it!

The data will now be sent to your webhook in the body of a post request. The data is a single json object representing an **unexpanded entity**.

Webhooks from **DELETE** do not send the entire object but only the **ID**.

> **Info:** **Tip!** Always validate your received data with you secret!

Webhooks have an `active` field, as you can see in the 
JSON response above, The service will keep the webhook active as long as
 the webhook url is capable of receiving data from the service. If the 
url fails **5** times the webhook will be set to inactive (`active: false`) and the service will stop to send data to this webhook.

Reactivating the webhook is done by re-registering it, this will update the `active` status to true.

> **Info:** **Tip!** Re-register your webhook on service start, to make sure it's always active!


```python
# Get ALL registered Webhooks
from requests import post
response = get('https://api.igdb.com/v4/webhooks/', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'}})
print ("response: %s" % str(response.json()))
```


### Viewing your webhooks

You can always get information about your webhooks from the API.
To get ALL of your registered webhooks simply send a `GET` request to `/webhooks`, without the endpoint. This will return a JSON array of your webhooks

To get information about a specific webhook you can make a `GET` request with the webhook id to `/webhooks/WEBHOOK_ID`, without the endpoint. This will return the webhook of that id.


```python
# Get ALL registered Webhooks
from requests import post
response = get('https://api.igdb.com/v4/webhooks/', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'}})
print ("response: %s" % str(response.json()))
```


### Removing a Webhook

To remove your existing webhook you need to send a `DELETE` request to `/webhooks/WEBHOOK_ID`, without the endpoint. The Webhook id is returned during the registration process or can be found with a `GET` request to `/webhooks/`.

The `DELETE` request will receive the deleted webhook as confirmation.


```python
from requests import post
response = post(
    'https://api.igdb.com/v4/ENDPOINT/webhooks/test/WEBHOOK_ID?entityId=ENTITY_ID',
    headers={
        'Client-ID': 'YOUR_CLIENT_ID',
        'Authorization': 'Bearer YOUR_ACCESS_TOKEN'
    }
)
```


### Testing

To make sure you have everything setup just right we have a test 
endpoint for the webhook service. This endpoint will send an object of 
your choosing to your newly created webhook.

Send a `POST` request to `ENDPOINT/webhooks/test/WEBHOOK_ID?entityId=ENTITY_ID`. The entity id is the id of the object from the endpoint you wish to test with, example:

`POST` to `games/webhooks/test/42?entityId=1337`:

This request will send the game object with id 1337 to your webhook url.


### Handling Webhooks on your end

When recieveing the webhook message on your end what we expect is to recieve a `200 OK` back within **15 seconds**.
 If the endpoint takes longer than 15 seconds to respond the event will 
be deemed as a failed event, fail 5 times and the webhook will be set to
 inactive.


# CORS Proxy


## CORS

If you intend to use our API from your website you will encounter an issue with security; namely CORS [Cross-Origin Resource Sharing](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS).

There are security mechanisms in place by all major browsers to stop 
websites from accessing other domains without getting explicit 
permission. This is done through HTTP headers. So, for example, 
amazinggameswebsite.com cannot access api.igdb.com without us explicitly
 stating in the HTTP headers (Access-Control-Allow-Origin) that they 
have permission.

We do not offer the configuration of these headers as a service, so 
any browser-based javascript and mobile javascript frameworks will not 
be able to communicate directly with the IGDB API.


### Workaround

See the guide for [setting up a proxy](#proxy) or set up a proxy using [CORS Anywhere](https://github.com/Rob--W/cors-anywhere)


## Proxy

There are a number of reasons why you may wish to proxy requests to the IGDB API.

- To have a backend that keeps track of your Oauth Application Tokens
- Caching requests to the API for better performance
- Enable application logging to track/debug usage
- Enable CORS between the proxy and applications


### How do I set up a proxy?

Proxies can be complex, but to get you started we have a simple guide to get you up and running quickly through AWS.

We have provided a single link that will let you deploy an AWS Api 
Gateway in your own AWS account that will serve as a proxy. This Stack 
will also handle your Access Token rotations automatically for you, so 
you don’t need to think about that.


### What will it cost?

AWS has a [very generous free-tier](https://aws.amazon.com/api-gateway/pricing/) for new users and the services used in the provided solution ([Api Gateway](https://aws.amazon.com/api-gateway/pricing/), [Secrets Manager](https://aws.amazon.com/secretsmanager/pricing/), [Lambda](https://aws.amazon.com/lambda/pricing/)). Please use the [AWS Pricing Calculator](https://calculator.aws/#/estimate) to gauge how much this will cost you before setting up your Stack.


### Stack Setup

**Prerequisites:** You need to have an AWS account with permissions to deploy CloudFormation stacks.

1. Click this [link](https://console.aws.amazon.com/cloudformation/home?region=us-west-2#/stacks/create/review?stackName=simple-igdb-proxy&templateURL=https://api-simple-proxy-bucket.s3.amazonaws.com/api-simple-proxy.yml) to get started.
2. Go over the Stack Details

You have to agree to the terms and conditions.
You have to fill in your Twitch Application Credentials
It’s recommended to protect your proxy by enabling Api Keys
NOTE: Enabling Caching will come with extra costs as this is NOT covered by the Free-tier
NOTE: Enabling CORS will ‘break’ Protobuf responses, some libraries might not work.
3. Click Next
4. Configure Stack Options - Nothing is required here, you can click Next
5. Verify Settings, click the checkbox at the bottom, then click “Create Stack”
6. You will now see the “Stack Details” screen, hit the refresh arrow 
button on the right until your stack name on the left says 
“UPDATE_COMPLETE”
7. Click on the “Outputs” tab to get the URL to your new proxy.

The “Resources” tab summarises all the services deployed on your account.
The “Template” tab displays the template used for deployment.
8. You can now post requests to your URL and it will proxy to our API

If you enabled Api Keys you will need to specify the header x-api-key and the key can be found via a link through the “Resources” tab for “ApiDefaultKey”

**Important Note:** The url generated will end in `production`, so you will want to post to

`​​https://<your-api-gateway-unique-id>.execute-api.us-west-2.amazonaws.com/production/v4/games`


### What’s next?

You can do a lot of things via [API Gateway](https://docs.aws.amazon.com/apigateway/latest/developerguide/getting-started.html).

- You can improve the security of your proxy by creating another sort of [Authentication](https://docs.aws.amazon.com/apigateway/latest/developerguide/apigateway-integrate-with-cognito.html), to prevent others from using up your RPS quota.
- You can also setup your own Domain name and SSL with [Route53](https://docs.aws.amazon.com/apigateway/latest/developerguide/how-to-custom-domains.html)
- You can modify the path of the proxy to have it serve as the front-end to your own APIs

Perform a calculation? Lambda Integration
Just want to store some records? DynamoDB Integration
Want users to be able to upload/download files? S3 Integration
- Enable request [logging](https://docs.aws.amazon.com/apigateway/latest/developerguide/security-monitoring.html)


### Alternatives

- CORS: Setup a proxy using [CORS Anywhere](https://github.com/Rob--W/cors-anywhere)


# Reference


## Images

> **Info:** **Note:** Images that are removed or replaced from IGDB.com exist for **30 days** before they are removed. Keep that in mind when designing cache logic.


### Examples

- Address:

https://api.igdb.com/v4/games/
- Body:

fields screenshots.*;
where id = 1942;

Here we retrieve the image properties of the game with the id “1942”


```json
[{
	"id": 1942,
	"screenshots": [{
			"id": 9742,
			"game": 1942,
			"height": 1080,
			"image_id": "mnljdjtrh44x4snmierh",
			"url": "//images.igdb.com/igdb/image/upload/t_thumb/mnljdjtrh44x4snmierh.jpg",
			"width": 1920
		},
		{
			"id": 9743,
			"game": 1942,
			"height": 1080,
			"image_id": "em1y2ugcwy2myuhvb9db",
			"url": "//images.igdb.com/igdb/image/upload/t_thumb/em1y2ugcwy2myuhvb9db.jpg",
			"width": 1920
		}
	]
}]
```

**Response example on the right –>**

**Image url structure:**

`https://images.igdb.com/igdb/image/upload/t_screenshot_med_2x/dfgkfivjrhcksyymh9vw.jpg`

**Break down:**

`https://images.igdb.com/igdb/image/upload/t_{size}/{hash}.jpg`

`size` is one of the interchangeable size types listed below.
`hash` is the id of the image.
The image sizes are all maximum size but by appending `_2x` to any size, you can get retina (DPR 2.0) sizes (`cover_small_2x`).


| Name | Size | Extra |
| --- | --- | --- |
| cover_small | 90 x 128 | Fit |
| screenshot_med | 569 x 320 | Lfill, Center gravity |
| cover_big | 264 x 374 | Fit |
| logo_med | 284 x 160 | Fit |
| screenshot_big | 889 x 500 | Lfill, Center gravity |
| screenshot_huge | 1280 x 720 | Lfill, Center gravity |
| thumb | 90 x 90 | Thumb, Center gravity |
| micro | 35 x 35 | Thumb, Center gravity |
| 720p | 1280 x 720 | Fit, Center gravity |
| 1080p | 1920 x 1080 | Fit, Center gravity |


## Fields


### What?

Fields are properties of an entity. For example, a Game field would be `genres` or `release_dates`. Some fields have properties of their own, for example, the `genres` field has the property `name`.


### Where?

Fields can be used on any entity that has sub-properties such as Games, Companies, People etc.


### How?

Fields are requested in a comma separated list. For example, to get 
some information for some Games, Genres, Themes or anything else, you 
could request it like this:

**Apicalypse**

`where id = (4356,189,444);`

`fields name,release_dates,genres.name,rating`

**Legacy Parameters**

`/games/4356,189,444?fields=name,release_dates,genres.name,rating`

Note in Apicalypse the `name` property of `genres` can be accessed directly with a dot (genres.name).

A full list of fields can be obtained by passing a `*` as a field. Alternatively you can use the `meta` postfix: `/games/meta` to get a list of all fields.


### Shorthand

Another way of writing fields is to use the shorthand **f** which achieves the same result.

`f name,release_dates,genres.name,rating;`

`w id = (4356,189,444);`


## Exclude


### What?

Exclude is a complement to the regular fields which allows you to 
request all fields with the exception of any numbers of fields specified
 with exclude.


### How?

Fields to be excluded are specified as a comma separated list. For 
example, to get all fields excpect for screenshots, you could request it
 like this:

**Apicalypse**

`fields *;`

`exclude screenshots;`


### Shorthand

Another way of writing exclude is to use the shorthand **x** which achieves the same result.

`f *;`

`x screenshots;`


## Expander


### What?

Some fields are actually ids pointing to another endpoint. The 
expander feature is a convenient way to go into these other endpoints 
and access more information from them in the same query, instead of 
having to do multiple queries.


### Where?

Expands are specificed among the regular fields in the body of the query.


### How?

Fields can be expanded with a dot followed by the fields you want to access from a certain endpoint.


### Examples

In the example below we request the fields name and genres for the game The Witcher 3 with id 1942.

`fields name,genres;`

`where id = 1942;`

But this query will only return ids for the genres, which can be seen in the first response to the right:


```json
"First example response showing genre ids"
[
 {
 "id ": 1942,
 "genres":[
 12,
 31
 ],
 "name": "The Witcher 3: Wild Hunt"
 }
]
```

For some use cases the id is all 
that is needed, but other times more data is needed, This is when the 
expander features comes in handy.

`fields name,genres.name;`

`where id = 1942;`

This example with expander retrieves the name of each genre which can be seen in the second response to the right.


```json
"Second example response showing genre ids and name"
[
 {
 "id": 1942,
 "genres": [
 {
 "id": 12,
 "name": "Role-playing (RPG)"
 },
 {
 "id": 31,
 "name": "Adventure"
 }
 ],
 "name": "The Witcher 3: Wild Hunt"
 }
]
```

And lastly lets take a look at how you can use a wildcard character ***** to retrieve all data from genres in the previous example.

`fields name,genres.*;`

`where id = 1942;`

See the third response to the right where all available data for each genre is included in the response.


```json
"Third example response showing all available genre data"
[
 {
 "id": 1942,
 "genres": [
 {
 "id": 12,
 "created_at": 1297555200,
 "name": "Role-playing (RPG)",
 "slug": "role-playing-rpg",
 "updated_at": 1323216000,
 "url": "https://www.igdb.com/genres/role-playing-rpg"
 },
 {
 "id": 31,
 "created_at": 1323561600,
 "name": "Adventure",
 "slug": "adventure",
 "updated_at": 1323561600,
 "url": "https://www.igdb.com/genres/adventure"
 }
 ],
 "name": "The Witcher 3: Wild Hunt"
 }
]
```


## Filters


### What?

Filters are used to sift through results to get what you want. You 
can exclude and include results based on their properties. For example 
you could remove all Games where the `rating` was below 80 (`where rating >= 80`).


### How?

Filters are parameter arrays so must be added using special keys like this:

- Address:

https://api.igdb.com/v4/games/
- Body:

search “zelda”;
where rating >= 80 & release_dates.date > 631152000;


### Where?

Filters can be used on any entity that has sub-properties such as Games, Companies, People etc.


### Available Postfixes

- `=` Equal: Exact match equal.
- `!=` Not Equal: Exact match equal.
- `>` Greater than (works only on numbers).
- `>=` Greater than or equal to (works only on numbers).
- `<` Less than (works only on numbers).
- `<=` Less than or equal to (works only on numbers).
- `= "Your input string"*` Prefix: Exact match on the beginning of the string, can end with anything. (Case sensitive).
- `~ "Your input string"*` Prefix: Exact match on the beginning of the string, can end with anything. (Case insensitive).
- `= *"Your input string"` Postfix: Exact match at the end of the string, can start with anything. (Case sensitive).
- `~ *"Your input string"` Postfix: Exact match at the end of the string, can start with anything. (Case insensitive).
- `= *"Your input string"*` Infix Exact match in the middle of the string, can start and end with anything. (Case sensitive).
- `~ *"Your input string"*` Infix Exact match in the middle of the string, can start and end with anything. (Case insensitive).
- `!= null` The value is not null.
- `= null` The value is null.
- `[V1,V2,...Vn]` The value exists within the (comma separated) array (AND between values).
- `![V1,V2,...Vn]` The values must not exist within the (comma separated) array (AND between values).
- `(V1,V2,...Vn)` The value has any within the (comma separated) array (OR between values).
- `!(V1,V2,...Vn)` The values must not exist within the (comma separated) array (OR between values).
- `{V1,V2,...V2}` Exact match on arrays. (Does not work on ids, strings, etc).


### Examples

**Filter by multiple platforms**

To get games that are released on PS4 OR XBOX ONE OR PC

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where release_dates.platform = (48,49,6);

Similarly if you want games released on PS4 AND XBOX ONE AND PC

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where release_dates.platform = [48,49,6];

If you want games released only on PC

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where release_dates.platform = 6;

And if you want games released for PC OR any other platform

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where release_dates.platform = (6);


### Combining Multiple Filters

It is possible to to use logical operators between filters, which could look something like this:

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name,platforms,genres.name;
where (platforms = [6,48] & genres = 13) | (platforms = [130,48] & genres = 12);

The response from this example query will be games that fulfil one or both of two sets or requirements:

- Games released for for both PC (6), and PS4 (48) and also has the genre simulator (13).
- Games released for for both Switch (130), and PS4 (48) and also has the genre Role-Playing (13).


### Prefix, Postfix and Infix

**Prefix**

Filtering for game names beginning with “Super”
(this will return games such as for example Super Mario World)

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where name = "Super"*;

**Postfix**

Filtering for game names ending with with “World”
(this will also return games such as for example Super Mario World)

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where name = *"World";

**Infix**

Filtering for game names containing the string “Smash” anywhere
(this will return games such as for example Super Smash Bros)

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where name = *"Smash"*;

**case insensitive version**

Filtering for game names containing the string “Smash”
(this will return games such as for example Super Smash Bros)

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where name ~ *"Smash"*;


### Removing erotic games from API responses

Some queries may return games with erotic themes. All erotic
games in the database has the theme ’erotic’ (id = 42).
So by adding a simple filter like the one below you can remove them
from your responses.

- Address:

https://api.igdb.com/v4/games/
- Body:

fields name;
where themes != (42);


## Sorting


### What?

Sorting is used to order results by a specific field.


### How?

You can order results like this:

- Address:

https://api.igdb.com/v4/games/
- Body:

sort release_dates.date desc; 
where rating >= 80;

Notice the appended `:desc` (descending) which could also be `:asc` (ascending) if required.

**Order by rating**

Rating parameter for games. You can access it like this:

- Address:

 https://api.igdb.com/v4/games/
- Body:

fields name,rating; 
sort rating desc; 
where rating != null;


### Where?

Ordering can be used on any entity.


## Search


### What?

Search based on name, results are sorted by similarity to the given search string.


### Where?

Searchable endpoints:

- Characters
- Collections
- Games
- People
- Platforms
- Themes


### How?

You specify which endpoint to search through in the Address field of your request.
The search string is then entered in the body of the request by typing `search`,
blank space followed by the string you wish to search for.

- Address:

https://api.igdb.com/v4/games/
- Body:

search “zelda”;


## Pagination

Here is an example for how to use **limit**. The default limit is 10. The maximum value you can set for limit is 500.

- Address:

https://api.igdb.com/v4/platforms/
- Body:

limit 33;

There is also an **offset**. This will start the list at position 22 and give 33 results.

- Address:

https://api.igdb.com/v4/platforms/
- Body:

limit 33; 
offset 22;


## Protocol Buffers

[Google Protocol Buffers](https://developers.google.com/protocol-buffers) is a language neutral method for serializing structured data.

The IGDB API supports responses in this format so you do not have to 
write your own serialization libraries, but instead you could just 
generate one.

Since this is langage neutral it is supported by a variatey of languages.


### How?

Generate the objects in your language of choise with our own Protobuf file, [here](https://api.igdb.com/v4/igdbapi.proto)

This file contains the mapping of the entire IGDB API and can be used to
 generate wrappers, code and tooling in any programming language.

The protobuf file is created in accordance with the [proto3 specification](https://developers.google.com/protocol-buffers/docs/proto3)

There are plenty of examples on how to do this Online and on the [Protobuf Site](https://developers.google.com/protocol-buffers/docs/tutorials).


### Where?

To start recieving protobuf compatible responses from then api all you need to do is add `.pb` at the end of your request:

`https://api.igdb.com/v4/games.pb`

Then use your generated files to parse the response into the expected object.


## Tag Numbers

Tag numbers are automatically generated numbers which provide a 
compact and fast way to do complex filtering on the IGDB API. The number
 calculation can be easily achieved with any programming language.

The basis of the calculation is a 32bit integer, where the first 4 
bits contain the object type ID, and the remaining 28 bits represent the
 ID of the object we are generating the tag number for.

Using this method a flat index of custom object ‘hashes’ can be 
maintained in which index the search and filtering is faster than using 
conventional methods.

Currently the following object types use tags:


| Type ID | Name |
| --- | --- |
| 0 | Theme |
| 1 | Genre |
| 2 | Keyword |
| 3 | Game |
| 4 | Player Perspective |

Let’s see two examples for tag number calculation.

We try to find all the games 
which relate to the Shooter genre. The tag number generation in 
Javascript would look something like the example on the right.

Javascript example query:

- Address:

https://api.igdb.com/v4/games/
- Body:

where tags = (268435461);

Python example query:

- Address:

https://api.igdb.com/v4/games/
- Body:

where tags = (536871060);


## Multi-Query

Multi-Query is a new way to request a huge amount of information in one request!
With Multi-Query you can request multiple endpoints at once, it also works with multiple requests to a single endpoint as well.

A Multi-Query is made by making a `POST` request to: `https://api.igdb.com/v4/multiquery`.

**Syntax Structure**
The Multi-Query syntax is made up of three pieces; “Endpoint name”, 
“Result Name (Given by you)”, and the APICalypse query inside the body 
{}.

**important**
You can only run a maximum of 10 queries.


### Example 1:

Get the count of platforms in the api.


```
query platforms/count "Count of Platforms" {
 // here we can have additional filters
};
```

This above query will give us the following result:


```json
[
 {
 "name": "Count of Platforms",
 "count": 155
 }
]
```


### Example 2:

Get Playstation 4 Exclusives


```
query games "Playstation Games" {
	fields name,platforms.name;
	where platforms !=n & platforms = {48};
	limit 1;
};
```

This above query will give us the following result:


### Example 3:

Combining the queries of example 1 and 2.


```
query platforms/count "Count of Platforms" {
 // here we can ahve additional filters
};

query games "Playstation Games" {
	fields name,platforms.name;
	where platforms !=n & platforms = {48};
	limit 1;
};
```


## APICalypse


### APICalypse cheatsheet

APICalypse is a new language used for this api which greatly 
simplifies how you can query your requests compared to the url 
parameters used in API V2.


### Fields

Fields are used to select which fields you want back from your request to the api.

To select fields you need the APICalypse command `fields` or its shorthand `f`.

Popular wildcard is to add `*` instead of a field, this will give you all of the fields.

`fields name,release_dates,genres.name,rating;`

`f name,release_dates,genres.name,rating;`


### Exclude

Commonly used with selecting all fields with the wildcard `*` this command will exclude the fields that you select.

To exclude fields you don’t need the APICalypse command `exclude` or its shorthand `x`.

`fields *;`

`exclude tags,keywords;`

`f *;`

`x tags,keywords;`


### Where

Where is easiest described as a filter. With where you can filter on specific fields.

To filter your results use the APICalypse command `where` or its shorthand `w`.

`fields *;`

`where genres = 4;`

`f *;`

`w genres = 4;`


### Limit

Limit describes how many results you will get back from the api, the standard value is 10.

To set a new limit use the APICalypse command `limit` or it’s shorthand `l`.

`fields *;`

`limit 50;`

`f *;`

`l 50;`


### Offset

Offset describes how many results you will skip over, standard is 0.

To set a new offset use the APICalypse command `offset` or it’s shorthand `o`.

Offset is often used together with Limit for pagination.

`limit 50;`

`offset 50;`

`l 50;`

`o 50;`


### Sort

Use Sort to order the results to your liking.

To order the results use the APICalypse command `sort` or it’s shorthand `s`.

Sort has two accompaning commands for “direction”; `asc` Ascending order and `desc` Decending order.

`fields *;`

`sort rating asc;`

`f *;`

`s rating desc;`


### Search

To find a specific title you can use Search.

To use search use the APICalypse command `search`, it has no shorthand :(.
Search has it’s own endpoint where it is good to use a filter for specific kinds of results, example `where game != null;` for only games.

`search "Halo";`

`fields name;`

`search "Halo";`

`f name;`


### Other shorts

Null can be written `null` or `n`.
Booleans can be written as `true` or `t` and `false` or `f`


# Migration Enums to Tables


## Important Changes Coming to the IGDB API

We’re announcing upcoming changes to the IGDB API that will affect 
how certain data fields are structured and accessed. These changes are 
designed to make our gaming database more dynamic and better suited to 
the evolving gaming industry.


### Key Changes

We’re moving away from using static enum values in our API to using 
more flexible table-based structures. While this is primarily an 
internal change, it affects how certain fields are named in our API. To 
ensure a smooth transition:

- All current enum values will remain the same in the new table structure
- Only the field names are changing, not the values they contain

NOTE: Changes are expected for the Age Rating Categories, IDs will not reflect the same enum values here.
- This change will allow us to evolve our data structure more efficiently in the future
- This change will allow you to expand these fields instead of keeping hardcoded enums


## What is Changing?

Several field names are being standardized across our endpoints for better clarity and consistency:


| Endpoint | Current Field | New Field |
| --- | --- | --- |
| age_rating | category | organization |
| age_rating | rating | rating_category |
| character | gender | character_gender |
| character | species | character_species |
| companies | change_date_category | change_date_format |
| companies | start_date_category | start_date_format |
| company_website | category | type |
| external_game | category | external_game_source |
| external_game | media | game_release_format |
| platform | category | platform_type |
| website | category | type |

Additionally, we’re adding some new fields:

- The `age_rating` endpoint will now use `rating_content_descriptions` instead of `content_descriptions` (replacing age_rating_content_descriptions with age_rating_content_descriptions_v2)
- The `companies` endpoint will have a `status` field for active,defunct,merged, or renamed.

**New Endpoints replacing enum values**

- age_rating_organizations
- age_rating_categories
- age_rating_content_descriptions_v2
- character_genders
- character_species
- company_status
- company_websites
- date_formats
- external_game_sources
- game_release_formats
- game_status
- game_types
- platform_types
- release_date_regions
- website_types

**Datadumps**

All of these changes will be reflected in the daily data dumps.


## Migration Timeline

- Migration Period: starting on February 18 to August 31 (6 months)
- During this period, both old and new field names will be available
- Monthly reminders will be sent to all API users about the upcoming deprecation
- After the migration period, the old field names will be removed


### How Does This Affect You?

If your application uses any of the fields listed above, you’ll need 
to update your code to use the new field names within the next 6 months.
 The current field names will be removed after the migration period 
ends.


### Migration Recommendations

1. Begin updating your applications to use the new field names as soon as possible
2. Test your applications thoroughly with the new field names
3. Complete all necessary changes before the end of the 6 month migration period
4. Keep an eye on Discord for monthly deprecation reminders


## Questions or Concerns?

If you have any questions about these changes or need assistance with migration, please:

- Review our updated [API documentation](https://api-docs.igdb.com/#getting-started)
- Reach out to our support team
- Join our [discord](https://discord.gg/igdb) community for discussions

We’re committed to making this transition as smooth as possible for all our API users.


# Partnership

Interested about using the API for a commercial project? No problem, 
we allow commercial usage. Get in touch with us about our partner 
program!


### How?

To register for a commertial agreement reach out to [partner@igdb.com](mailto:partner@igdb.com)


### Exclusive Features

Automatic data dumps every 24 hours.

*More features coming soon.*


## Data Dumps

All [endpoints](#endpoints) are available as CSV Data Dumps!

Daily updated CSV Data Dumps which can be used to kick start your projects or keep your data up to date (within 24 hours).

> **Info:** *Please note that data dumps are exclusively available to our Data Partners.*


```python
from requests import post
response = get('https://api.igdb.com/v4/dumps', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'}})
print ("response: %s" % str(response.json()))
```


### Listing dumps

To list the available data dumps make a GET request to `/dumps`.

This will return a list of available Data Dumps describing the `endpoint`, `file name`, and `updated at`.


```yaml
# Example response from /dumps
[
 {
		"endpoint": "games",
		"file_name": "1234567890_games.csv",
		"updated_at": 1234567890
	}
]
```


```python
from requests import post
response = get('https://api.igdb.com/v4/dumps', **{'headers': {'Client-ID': 'Client ID', 'Authorization': 'Bearer access_token'}})
print ("response: %s" % str(response.json()))
```


### Downloading CSV

To get the download link for the csv files make a GET request to `/dumps/ENDPOINT`
The response object will contain the download link for the CSV and the schema version & schema JSON structure of the data.

**S3 Download Url:**

The download Url is a presigned S3 url that is valid for 5 minutes.

**Schema**

The `schema_version` and `schema` will reflect the current data structure and data type that the Dump is using.

The schema version number will change when the `schema` changes, so if you are planning on an automated setup for this you will need to keep this in mind.


```yaml
# Example response from /dumps/games
{
	"s3_url": "S3_DOWNLOAD_URL",
	"endpoint": "games",
	"file_name": "1234567890_games.csv",
	"size_bytes": 123456789,
	"updated_at": 1234567890,
	"schema_version": "1234567890",
	"schema": {
		"id": "LONG",
		"name": "STRING",
		"url": "STRING",
 "franchises": "LONG[]",
 "rating": "DOUBLE",
 "created_at": "TIMESTAMP",
 "checksum": "UUID",
	}
}
```


# FAQ


## Business related FAQ


### 1. I want to use the API for a commercial project, is it allowed?

Yes, we offer commercial partnerships for users looking to integrate 
the API in monetized products. From our side, as part of the 
partnership, we ask for user facing attribution to IGDB.com from 
products integrating the IGDB API.

For more details on that process, please reach out to [partner@igdb.com](mailto:partner@igdb.com)


### 2. What is the price of the API?

The API is free for both non-commercial and commercial projects.


### 3. Am I allowed to store/cache the data locally?

Yes. In fact, we prefer if you store and serve the data to your end 
users. You remain in control over your user experience, while 
alleviating pressure on the API itself.


### 4. Regarding user facing attribution (relating to the commercial partnership), any specific guidelines?

Not really. We expect fair attribution, i.e. attribution that is 
visible to your users and located in a static location (e.g. not in a 
change log).


### 5. What happens with the data retrieved, in the case of partnership termination?

You are allowed to keep all data you retrieve from the API and we 
will not ask you to remove the data in case of partnership termination.


### 6. We don’t wish to attribute IGDB.com as part of the partnership. Are there any other way?

Yes. If you have data that we think will complete the overall IGDB 
data set and you are willing to share that data with us, we can opt for 
this approach instead. Please be aware, however, that we are only 
interested in publicly available data that we can re-distribute using 
this API.


## Technical related FAQ


### 1. Can I use Twitch User Credentials to access the API?

The IGDB API uses `Application Credentials` to authenticate, you **cannot** use user credentials to authenticate API requests

More information about authentication can be found in the documentation, [here](#authentication)


### 2. The requested images are in the wrong format!

Requesting images using the API returns a default image url using the `t_thumb` format.
To request larger image sizes you should manually create your own image url using the `image_id` and the appropriate image size.
example: `https://images.igdb.com/igdb/image/upload/t_{size}/{image_id}.png`

More information about images and image sizes can be found in our documentation, [here](#images)


### 3. Why am I recieving a CORS error?

The IGDB API does not support browser requests, CORS, for security 
reasons. This is because the request would leak your access token! We 
suggest that you create a backend proxy which authenticates and queries 
the API directly, and can be set up as a trusted connection for your 
client application.

For more information see our documentation, [here](#proxy)


### 4. My AccessToken stopped working, why?

Your Access Token is only active for **60 days** and your application can only have **25 active** Access Tokens at one time, going over this limit starts to inactivate older tokens.


### 5. Why am I only receiving IDs?

An empty request will only yield a list of IDs. To request more information in a single request you should *expand* your request.

Ex: `fields *, cover.*;`

More information about expanding requests, [here](#expander)

More example requests, [here](#examples)


### 6. Why am I only receiving 10 items, how do I get more?

The default item limit is set to 10. To edit this limit simply specify the limit in your request.

Ex: `limit 50;`

The maximum limit is set to 500 items/request.

Read more about query syntax, [here](#apicalypse-1)


# Support


### Have a question?

If you have any questions about the API we recommend that you [join our Discord](https://discord.gg/igdb),
there you can discuss the API with other people working with it as well as the developers of the API and ask questions.


### Reporting a bug

If you would like to report a bug you can do so in Discord or use [Uservoice](https://twitch.uservoice.com/forums/929953-igdb?category_id=389365)


# License

Any code examples or snippets found under [api-docs.igdb.com](https://api-docs.igdb.com/) are made available under the [Twitch Developer Services Agreement](https://www.twitch.tv/p/legal/developer-agreement/) as Program Materials.

