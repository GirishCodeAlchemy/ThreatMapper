/* tslint:disable */
/* eslint-disable */
/**
 * Deepfence ThreatMapper
 * Deepfence Runtime API provides programmatic control over Deepfence microservice securing your container, kubernetes and cloud deployments. The API abstracts away underlying infrastructure details like cloud provider,  container distros, container orchestrator and type of deployment. This is one uniform API to manage and control security alerts, policies and response to alerts for microservices running anywhere i.e. managed pure greenfield container deployments or a mix of containers, VMs and serverless paradigms like AWS Fargate.
 *
 * The version of the OpenAPI document: 2.0.0
 * Contact: community@deepfence.io
 *
 * NOTE: This class is auto generated by OpenAPI Generator (https://openapi-generator.tech).
 * https://openapi-generator.tech
 * Do not edit the class manually.
 */


import * as runtime from '../runtime';
import type {
  ApiDocsBadRequestResponse,
  ApiDocsFailureResponse,
  GraphProviderThreatGraph,
} from '../models';
import {
    ApiDocsBadRequestResponseFromJSON,
    ApiDocsBadRequestResponseToJSON,
    ApiDocsFailureResponseFromJSON,
    ApiDocsFailureResponseToJSON,
    GraphProviderThreatGraphFromJSON,
    GraphProviderThreatGraphToJSON,
} from '../models';

/**
 * ThreatApi - interface
 * 
 * @export
 * @interface ThreatApiInterface
 */
export interface ThreatApiInterface {
    /**
     * Retrieve the full threat graph associated with the account
     * @summary Get Threat Graph
     * @param {*} [options] Override http request option.
     * @throws {RequiredError}
     * @memberof ThreatApiInterface
     */
    getThreatGraphRaw(initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<{ [key: string]: GraphProviderThreatGraph; }>>;

    /**
     * Retrieve the full threat graph associated with the account
     * Get Threat Graph
     */
    getThreatGraph(initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<{ [key: string]: GraphProviderThreatGraph; }>;

}

/**
 * 
 */
export class ThreatApi extends runtime.BaseAPI implements ThreatApiInterface {

    /**
     * Retrieve the full threat graph associated with the account
     * Get Threat Graph
     */
    async getThreatGraphRaw(initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<runtime.ApiResponse<{ [key: string]: GraphProviderThreatGraph; }>> {
        const queryParameters: any = {};

        const headerParameters: runtime.HTTPHeaders = {};

        if (this.configuration && this.configuration.accessToken) {
            const token = this.configuration.accessToken;
            const tokenString = await token("bearer_token", []);

            if (tokenString) {
                headerParameters["Authorization"] = `Bearer ${tokenString}`;
            }
        }
        const response = await this.request({
            path: `/deepfence/graph/threat`,
            method: 'POST',
            headers: headerParameters,
            query: queryParameters,
        }, initOverrides);

        return new runtime.JSONApiResponse(response, (jsonValue) => runtime.mapValues(jsonValue, GraphProviderThreatGraphFromJSON));
    }

    /**
     * Retrieve the full threat graph associated with the account
     * Get Threat Graph
     */
    async getThreatGraph(initOverrides?: RequestInit | runtime.InitOverrideFunction): Promise<{ [key: string]: GraphProviderThreatGraph; }> {
        const response = await this.getThreatGraphRaw(initOverrides);
        return await response.value();
    }

}