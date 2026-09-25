CREATE TABLE ost_ticket_priority (
  priority_id tinyint(4) NOT NULL AUTO_INCREMENT,
  priority varchar(60) NOT NULL DEFAULT '',
  priority_desc varchar(30) NOT NULL DEFAULT '',
  priority_color varchar(7) NOT NULL DEFAULT '',
  priority_urgency tinyint(1) unsigned NOT NULL DEFAULT 0,
  ispublic tinyint(1) NOT NULL DEFAULT 1,
  PRIMARY KEY (priority_id)
);
CREATE TABLE ost_ticket_status (
  id int(11) NOT NULL AUTO_INCREMENT,
  name varchar(60) NOT NULL DEFAULT '',
  state varchar(16) DEFAULT NULL,
  mode int(11) unsigned NOT NULL DEFAULT 0,
  flags int(11) unsigned NOT NULL DEFAULT 0,
  sort int(11) unsigned NOT NULL DEFAULT 0,
  properties text NOT NULL,
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_department (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  pid int(11) unsigned DEFAULT NULL,
  manager_id int(10) unsigned NOT NULL DEFAULT 0,
  flags int(10) unsigned NOT NULL DEFAULT 0,
  name varchar(128) NOT NULL DEFAULT '',
  signature text NOT NULL,
  ispublic tinyint(1) unsigned NOT NULL DEFAULT 1,
  updated datetime NOT NULL,
  created datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_staff (
  staff_id int(11) unsigned NOT NULL AUTO_INCREMENT,
  dept_id int(10) unsigned NOT NULL DEFAULT 0,
  role_id int(10) unsigned NOT NULL DEFAULT 0,
  username varchar(32) NOT NULL DEFAULT '',
  firstname varchar(32) DEFAULT NULL,
  lastname varchar(32) DEFAULT NULL,
  passwd varchar(128) DEFAULT NULL,
  backend varchar(32) DEFAULT NULL,
  email varchar(255) DEFAULT NULL,
  signature text NOT NULL,
  isactive tinyint(1) NOT NULL DEFAULT 1,
  isadmin tinyint(1) NOT NULL DEFAULT 0,
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (staff_id)
);
CREATE TABLE ost_staff_dept_access (
  staff_id int(10) unsigned NOT NULL DEFAULT 0,
  dept_id int(10) unsigned NOT NULL DEFAULT 0,
  role_id int(10) unsigned NOT NULL DEFAULT 0,
  flags int(10) unsigned NOT NULL DEFAULT 1,
  PRIMARY KEY (staff_id, dept_id)
);
CREATE TABLE ost_help_topic (
  topic_id int(11) unsigned NOT NULL AUTO_INCREMENT,
  topic_pid int(10) unsigned NOT NULL DEFAULT 0,
  ispublic tinyint(1) unsigned NOT NULL DEFAULT 1,
  flags int(10) unsigned DEFAULT 0,
  status_id int(10) unsigned NOT NULL DEFAULT 0,
  priority_id int(10) unsigned NOT NULL DEFAULT 0,
  dept_id int(10) unsigned NOT NULL DEFAULT 0,
  staff_id int(10) unsigned NOT NULL DEFAULT 0,
  sort int(10) unsigned NOT NULL DEFAULT 0,
  topic varchar(128) NOT NULL DEFAULT '',
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (topic_id)
);
CREATE TABLE ost_user (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  org_id int(10) unsigned NOT NULL DEFAULT 0,
  default_email_id int(10) NOT NULL DEFAULT 0,
  status int(11) unsigned NOT NULL DEFAULT 0,
  name varchar(128) NOT NULL,
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_user_email (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  user_id int(10) unsigned NOT NULL,
  flags int(10) unsigned NOT NULL DEFAULT 0,
  address varchar(255) NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_form (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  pid int(10) unsigned DEFAULT NULL,
  type varchar(8) NOT NULL DEFAULT 'G',
  flags int(10) unsigned NOT NULL DEFAULT 1,
  title varchar(255) NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_form_field (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  form_id int(10) unsigned NOT NULL,
  flags int(10) unsigned DEFAULT 1,
  type varchar(255) NOT NULL DEFAULT 'text',
  label varchar(255) NOT NULL,
  name varchar(64) NOT NULL,
  sort int(11) unsigned NOT NULL DEFAULT 0,
  PRIMARY KEY (id)
);
CREATE TABLE ost_form_entry (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  form_id int(11) unsigned NOT NULL,
  object_id int(11) unsigned DEFAULT NULL,
  object_type char(1) NOT NULL DEFAULT 'T',
  sort int(11) unsigned NOT NULL DEFAULT 1,
  PRIMARY KEY (id)
);
CREATE TABLE ost_form_entry_values (
  entry_id int(11) unsigned NOT NULL,
  field_id int(11) unsigned NOT NULL,
  value text,
  value_id int(11),
  PRIMARY KEY (entry_id, field_id)
);
CREATE TABLE ost_ticket (
  ticket_id int(11) unsigned NOT NULL AUTO_INCREMENT,
  ticket_pid int(11) unsigned DEFAULT NULL,
  number varchar(20),
  user_id int(11) unsigned NOT NULL DEFAULT 0,
  user_email_id int(11) unsigned NOT NULL DEFAULT 0,
  status_id int(10) unsigned NOT NULL DEFAULT 0,
  dept_id int(10) unsigned NOT NULL DEFAULT 0,
  sla_id int(10) unsigned NOT NULL DEFAULT 0,
  topic_id int(10) unsigned NOT NULL DEFAULT 0,
  staff_id int(10) unsigned NOT NULL DEFAULT 0,
  team_id int(10) unsigned NOT NULL DEFAULT 0,
  email_id int(11) unsigned NOT NULL DEFAULT 0,
  flags int(10) unsigned NOT NULL DEFAULT 0,
  ip_address varchar(64) NOT NULL DEFAULT '',
  source enum('Web','Email','Phone','API','Other') NOT NULL DEFAULT 'Other',
  source_extra varchar(40) DEFAULT NULL,
  isoverdue tinyint(1) unsigned NOT NULL DEFAULT 0,
  isanswered tinyint(1) unsigned NOT NULL DEFAULT 0,
  duedate datetime DEFAULT NULL,
  closed datetime DEFAULT NULL,
  lastupdate datetime DEFAULT NULL,
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (ticket_id)
);
CREATE TABLE ost_thread (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  object_id int(11) unsigned NOT NULL,
  object_type char(1) NOT NULL,
  extra text,
  lastresponse datetime DEFAULT NULL,
  lastmessage datetime DEFAULT NULL,
  created datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_thread_entry (
  id int(11) unsigned NOT NULL AUTO_INCREMENT,
  pid int(11) unsigned NOT NULL DEFAULT 0,
  thread_id int(11) unsigned NOT NULL DEFAULT 0,
  staff_id int(11) unsigned NOT NULL DEFAULT 0,
  user_id int(11) unsigned NOT NULL DEFAULT 0,
  type char(1) NOT NULL DEFAULT '',
  flags int(11) unsigned NOT NULL DEFAULT 0,
  poster varchar(128) NOT NULL DEFAULT '',
  source varchar(32) NOT NULL DEFAULT '',
  title varchar(255),
  body text NOT NULL,
  format varchar(16) NOT NULL DEFAULT 'html',
  ip_address varchar(64) NOT NULL DEFAULT '',
  created datetime NOT NULL,
  updated datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_attachment (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  object_id int(11) unsigned NOT NULL,
  type char(1) NOT NULL,
  file_id int(11) unsigned NOT NULL,
  name varchar(255) DEFAULT NULL,
  inline tinyint(1) unsigned NOT NULL DEFAULT 0,
  lang varchar(16),
  PRIMARY KEY (id)
);
CREATE TABLE ost_file (
  id int(11) NOT NULL AUTO_INCREMENT,
  ft char(1) NOT NULL DEFAULT 'T',
  bk char(1) NOT NULL DEFAULT 'D',
  type varchar(255) NOT NULL DEFAULT '',
  size bigint(20) unsigned NOT NULL DEFAULT 0,
  `key` varchar(86) NOT NULL,
  signature varchar(86) NOT NULL,
  name varchar(255) NOT NULL DEFAULT '',
  attrs varchar(255),
  created datetime NOT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_file_chunk (
  file_id int(11) NOT NULL,
  chunk_id int(11) NOT NULL,
  filedata longblob NOT NULL,
  PRIMARY KEY (file_id, chunk_id)
);
CREATE TABLE ost_event (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  name varchar(60) NOT NULL,
  description varchar(60) DEFAULT NULL,
  PRIMARY KEY (id)
);
CREATE TABLE ost_thread_event (
  id int(10) unsigned NOT NULL AUTO_INCREMENT,
  thread_id int(11) unsigned NOT NULL DEFAULT 0,
  thread_type char(1) NOT NULL DEFAULT '',
  event_id int(11) unsigned DEFAULT NULL,
  staff_id int(11) unsigned NOT NULL,
  team_id int(11) unsigned NOT NULL,
  dept_id int(11) unsigned NOT NULL,
  topic_id int(11) unsigned NOT NULL,
  data varchar(1024) DEFAULT NULL,
  username varchar(128) NOT NULL DEFAULT 'SYSTEM',
  uid int(11) unsigned DEFAULT NULL,
  uid_type char(1) NOT NULL DEFAULT 'S',
  annulled tinyint(1) unsigned NOT NULL DEFAULT 0,
  timestamp datetime NOT NULL,
  PRIMARY KEY (id)
);
