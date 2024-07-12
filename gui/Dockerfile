# Use an official Node.js runtime as a parent image
FROM node:18-alpine

# Set the working directory
WORKDIR /app

ARG NEXT_PUBLIC_DOCKER
ENV NEXT_PUBLIC_DOCKER=$NEXT_PUBLIC_DOCKER

# Copy package.json, package-lock.json, and next.config.mjs into the container
COPY package*.json next.config.mjs ./

# Install dependencies
RUN yarn install

# Copy the rest of your Next.js application
COPY . .

# Build your Next.js application
#RUN yarn build

# Start the Next.js application
#CMD ["yarn", "start"]
CMD ["yarn", "dev"]