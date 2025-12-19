#!/usr/bin/env node
import 'source-map-support/register';
import * as cdk from 'aws-cdk-lib';
import { RelayStack } from '../lib/relay-stack';
import { config } from '../config';

const app = new cdk.App();

new RelayStack(app, 'BacklogOAuthRelay', {
  config,
  env: {
    account: process.env.CDK_DEFAULT_ACCOUNT,
    region: process.env.CDK_DEFAULT_REGION ?? 'ap-northeast-1',
  },
  description: 'Backlog CLI OAuth Relay Server',
});
